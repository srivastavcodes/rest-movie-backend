package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func (bknd *backend) serve() error {
	srvr := &http.Server{
		ErrorLog:          slog.NewLogLogger(bknd.logger.Handler(), slog.LevelError),
		Handler:           bknd.routes(),
		Addr:              fmt.Sprintf(":%d", bknd.config.port),
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 3 * time.Second,
		IdleTimeout:       120 * time.Second,
		WriteTimeout:      10 * time.Second,
	}
	shutdownError := make(chan error)
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		sig := <-quit
		bknd.logger.Info("shutting down server", "signal", sig.String())

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		err := srvr.Shutdown(ctx)
		if err != nil {
			shutdownError <- err
		}
		bknd.logger.Info("completing background tasks", "addr", srvr.Addr)
		bknd.wtgrp.Wait()
		shutdownError <- nil
	}()
	bknd.logger.Info("server started", "addr", srvr.Addr, "env", bknd.config.env)
	err := srvr.ListenAndServe()
	if !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	err = <-shutdownError
	if err != nil {
		return err
	}
	bknd.logger.Info("server stopped", "addr", srvr.Addr, "env", bknd.config.env)
	return nil
}
