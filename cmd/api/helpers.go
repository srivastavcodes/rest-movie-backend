package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"FernArchive/internal/validator"

	"github.com/julienschmidt/httprouter"
)

type envelope map[string]any

func (bknd *backend) readIdParam(r *http.Request) (int64, error) {
	params := httprouter.ParamsFromContext(r.Context())
	id, err := strconv.ParseInt(params.ByName("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

func (bknd *backend) writeJSON(w http.ResponseWriter, status int, data envelope, headers http.Header) error {
	jsn, err := json.MarshalIndent(data, "", "\t")
	if err != nil {
		return err
	}
	jsn = append(jsn, '\n')
	for key, value := range headers {
		w.Header()[key] = value
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, err = w.Write(jsn)
	if err != nil {
		bknd.logger.Info("Failed to write response", "err", err)
	}
	return nil
}

func (bknd *backend) readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	var maxBytes int64 = 1_048_576
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	err := dec.Decode(dst)
	if err != nil {
		return bknd.decodeJSONError(err)
	}
	err = dec.Decode(&struct{}{})
	if !errors.Is(err, io.EOF) {
		return errors.New("body must contain exactly one JSON object")
	}
	return nil
}

func (bknd *backend) decodeJSONError(err error) error {
	var unmarshalTypeError *json.UnmarshalTypeError
	var syntaxError *json.SyntaxError
	var invalidUnmarshalError *json.InvalidUnmarshalError
	var maxBytesError *http.MaxBytesError

	switch {
	case errors.As(err, &syntaxError):
		return fmt.Errorf("body contains badly-formed JSON (at character %d)", syntaxError.Offset)
	case errors.Is(err, io.ErrUnexpectedEOF):
		return fmt.Errorf("body contains badly-formed JSON")
	case errors.Is(err, io.EOF):
		return fmt.Errorf("body must not be empty")
	case errors.As(err, &unmarshalTypeError):
		if unmarshalTypeError.Field != "" {
			return fmt.Errorf(
				"body contains incorrect JSON type for field %q", unmarshalTypeError.Field)
		}
		return fmt.Errorf("body contains incorrect JSON type (at character %d)", unmarshalTypeError.Offset)
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		fieldName := strings.TrimPrefix(err.Error(), "json: unknown field ")
		return fmt.Errorf("body contains unknown JSON field for key %s", fieldName)
	case errors.As(err, &maxBytesError):
		return fmt.Errorf("body must not be larger than %d bytes", maxBytesError.Limit)
	case errors.As(err, &invalidUnmarshalError):
		panic(err)
	default:
		return err
	}
}

func (bknd *backend) readCSV(qs url.Values, key string, defaultValue []string) []string {
	csv := qs.Get(key)
	if csv == "" {
		return defaultValue
	}
	return strings.Split(csv, ",")
}

func (bknd *backend) readString(qs url.Values, key, defaultValue string) string {
	str := qs.Get(key)
	if str == "" {
		return defaultValue
	}
	return str
}

func (bknd *backend) readInt(qs url.Values, key string, defaultValue int, vldtr *validator.Validator) int {
	str := qs.Get(key)
	if str == "" {
		return defaultValue
	}
	intgr, err := strconv.Atoi(str)
	if err != nil {
		vldtr.AddError(key, "must be an integer value")
		return defaultValue
	}
	return intgr
}

func (bknd *backend) background(fn func()) {
	bknd.wtgrp.Add(1)
	go func() {
		defer bknd.wtgrp.Done()
		defer func() {
			if err := recover(); err != nil {
				bknd.logger.Error(fmt.Sprintf("%v", err))
			}
		}()
		fn()
	}()
}
