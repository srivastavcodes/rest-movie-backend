package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"FernArchive/internal/data"
	"FernArchive/internal/validator"
)

func (bknd *backend) createAuthTokenHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := bknd.readJSON(w, r, &input); err != nil {
		bknd.badRequestResponse(w, r, err)
		return
	}
	vld := validator.NewValidator()

	data.ValidatePasswordPlainTxt(vld, input.Password)
	data.ValidateEmail(vld, input.Email)

	if !vld.Valid() {
		bknd.failedValidationResponse(w, r, vld.Errors)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	user, err := bknd.models.Users.GetByEmail(ctx, input.Email)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			bknd.deadlineExceededResponse(w, r)
		case errors.Is(err, context.Canceled):
			return
		case errors.Is(err, data.ErrRecordNotFound):
			bknd.invalidCredentialsResponse(w, r)
		default:
			bknd.serverErrorResponse(w, r, err)
		}
		return
	}
	correct, err := user.Password.CheckPass(input.Password)
	if err != nil {
		bknd.serverErrorResponse(w, r, err)
		return
	}
	if !correct {
		bknd.invalidCredentialsResponse(w, r)
		return
	}
	token, err := bknd.models.Tokens.NewToken(ctx, user.Id, 360*time.Hour, data.ScopeAuthentication)
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			bknd.deadlineExceededResponse(w, r)
		case errors.Is(err, context.Canceled):
			return
		default:
			bknd.serverErrorResponse(w, r, err)
		}
		return
	}
	err = bknd.writeJSON(w, http.StatusCreated, envelope{"authentication_token": token}, nil)
	if err != nil {
		bknd.serverErrorResponse(w, r, err)
	}
}
