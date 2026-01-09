package main

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"FernArchive/internal/data"
	"FernArchive/internal/validator"

	"github.com/tomasen/realip"
	"golang.org/x/time/rate"
)

func (bknd *backend) recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				w.Header().Set("Connection", "close")
				bknd.serverErrorResponse(w, r, fmt.Errorf("%s", err))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (bknd *backend) rateLimiter(next http.Handler) http.Handler {
	type client struct {
		limiter  *rate.Limiter
		lastSeen time.Time
	}
	var (
		mtx     sync.Mutex
		clients = make(map[string]*client)
	)
	go func() {
		for {
			time.Sleep(time.Minute)
			mtx.Lock()
			for ip, clnt := range clients {
				if time.Since(clnt.lastSeen) > 3*time.Minute {
					delete(clients, ip)
				}
			}
			mtx.Unlock()
		}
	}()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if bknd.config.limiter.enabled {
			ip := realip.FromRequest(r)
			mtx.Lock()
			if _, found := clients[ip]; !found {
				clients[ip] = &client{limiter: rate.NewLimiter(
					rate.Limit(bknd.config.limiter.rps), bknd.config.limiter.burst)}
			}
			clients[ip].lastSeen = time.Now()

			if !clients[ip].limiter.Allow() {
				mtx.Unlock()
				bknd.rateLimitExceededResponse(w, r)
				return
			}
			mtx.Unlock()
		}
		next.ServeHTTP(w, r)
	})
}

func (bknd *backend) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Authorization")

		authHeader := r.Header.Get("Authorization")

		if authHeader == "" {
			r = bknd.contextSetUser(r, data.AnonymousUser)
			next.ServeHTTP(w, r)
			return
		}
		headerParts := strings.SplitN(authHeader, " ", 2)

		if len(headerParts) != 2 || headerParts[0] != "Bearer" {
			bknd.invalidAuthTokenResponse(w, r)
			return
		}
		authToken := headerParts[1]
		vldtr := validator.NewValidator()

		if data.ValidateTokenPlainText(vldtr, authToken); !vldtr.Valid() {
			bknd.invalidAuthTokenResponse(w, r)
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		user, err := bknd.models.Users.GetForToken(ctx, data.ScopeAuthentication, authToken)
		if err != nil {
			switch {
			case errors.Is(err, data.ErrRecordNotFound):
				bknd.invalidAuthTokenResponse(w, r)
			default:
				bknd.serverErrorResponse(w, r, err)
			}
			return
		}
		r = bknd.contextSetUser(r, user)
		next.ServeHTTP(w, r)
	})
}

func (bknd *backend) requirePermission(code string, next http.HandlerFunc) http.HandlerFunc {
	fn := func(w http.ResponseWriter, r *http.Request) {
		user := bknd.contextGetUser(r)

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		permissions, err := bknd.models.Permissions.GetAllForUser(ctx, user.Id)
		if err != nil {
			bknd.serverErrorResponse(w, r, err)
			return
		}
		if !permissions.Include(code) {
			bknd.accessNotPermittedResponse(w, r)
			return
		}
		next.ServeHTTP(w, r)
	}
	return bknd.requireActivatedUser(fn)
}

func (bknd *backend) requireActivatedUser(next http.HandlerFunc) http.HandlerFunc {
	fn := func(w http.ResponseWriter, r *http.Request) {
		user := bknd.contextGetUser(r)
		if !user.Activated {
			bknd.inactiveAccountResponse(w, r)
			return
		}
		next.ServeHTTP(w, r)
	}
	return bknd.requireAuthenticatedUser(fn)
}

func (bknd *backend) requireAuthenticatedUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := bknd.contextGetUser(r)
		if user.IsAnonymous() {
			bknd.authRequiredResponse(w, r)
			return
		}
		next.ServeHTTP(w, r)
	}
}

func (bknd *backend) enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		w.Header().Add("Vary", "Access-Control-Request-Method")

		origin := r.Header.Get("Origin")
		if origin == "" {
			next.ServeHTTP(w, r)
			return
		}
		var originAllowed bool
		for _, allowedOrigin := range bknd.config.cors.allowedOrigins {
			if origin == allowedOrigin {
				originAllowed = true
				break
			}
		}
		if !originAllowed {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)

		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			w.Header().Set("Access-Control-Allow-Methods", "OPTIONS, PUT, PATCH, DELETE")
			w.Header().Set("Access-Control-Max-Age", "60")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type metricsResponseWriter struct {
	wrapped       http.ResponseWriter
	statusCode    int
	headerWritten bool
}

func newMetricsResponseWriter(w http.ResponseWriter) *metricsResponseWriter {
	return &metricsResponseWriter{
		wrapped:    w,
		statusCode: http.StatusOK,
	}
}

func (mrw *metricsResponseWriter) Header() http.Header {
	return mrw.wrapped.Header()
}

func (mrw *metricsResponseWriter) WriteHeader(statusCode int) {
	mrw.wrapped.WriteHeader(statusCode)

	if !mrw.headerWritten {
		mrw.statusCode = statusCode
		mrw.headerWritten = true
	}
}

func (mrw *metricsResponseWriter) Write(byt []byte) (int, error) {
	mrw.headerWritten = true
	return mrw.wrapped.Write(byt)
}

func (mrw *metricsResponseWriter) Unwrap() http.ResponseWriter {
	return mrw.wrapped
}

func (bknd *backend) requestMetrics(next http.Handler) http.Handler {
	var (
		requestsReceived           = expvar.NewInt("requests_received")
		processingTimeMicroseconds = expvar.NewInt("processing_time_ms")
		responsesSent              = expvar.NewInt("responses_sent")
		responsesSentByStatus      = expvar.NewMap("responses_sent_by_status")
	)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		requestsReceived.Add(1)

		mrw := newMetricsResponseWriter(w)
		next.ServeHTTP(mrw, r)

		responsesSent.Add(1)
		responsesSentByStatus.Add(strconv.Itoa(mrw.statusCode), 1)

		duration := time.Since(start).Microseconds()
		processingTimeMicroseconds.Add(duration)
	})
}
