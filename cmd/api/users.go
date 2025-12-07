package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"FernArchive/internal/data"
	"FernArchive/internal/validator"
)

func (bknd *backend) registerUserHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	err := bknd.readJSON(w, r, &input)
	if err != nil {
		bknd.badRequestResponse(w, r, err)
		return
	}
	user := &data.User{
		Name:      input.Name,
		Email:     input.Email,
		Activated: false,
	}
	err = user.Password.SetPass(input.Password)
	if err != nil {
		bknd.serverErrorResponse(w, r, err)
		return
	}
	vldtr := validator.NewValidator()
	data.ValidateUser(vldtr, user)

	if !vldtr.Valid() {
		bknd.failedValidationResponse(w, r, vldtr.Errors)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()

	err = bknd.models.Users.InsertUser(ctx, user)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrDuplicateEmail):
			vldtr.AddError("email", "email already taken")
			bknd.failedValidationResponse(w, r, vldtr.Errors)
		default:
			bknd.commonErrs(w, r, err)
		}
		return
	}
	err = bknd.models.Permissions.AddForUser(ctx, user.Id, "movie:user")
	if err != nil {
		bknd.commonErrs(w, r, err)
		return
	}
	token, err := bknd.models.Tokens.NewToken(ctx, user.Id, 3*24*time.Hour, data.ScopeActivation)
	if err != nil {
		bknd.commonErrs(w, r, err)
		return
	}
	bknd.background(func() {
		userData := map[string]any{"activationToken": token.PlainText,
			"userId":   user.Id,
			"username": user.Name,
		}
		err = bknd.mailer.SendEmail(user.Email, "user_welcome.gohtml", userData)
		if err != nil {
			bknd.logger.Error(err.Error())
		}
	})
	err = bknd.writeJSON(w, http.StatusAccepted, envelope{"user": user}, nil)
	if err != nil {
		bknd.serverErrorResponse(w, r, err)
	}
}

func (bknd *backend) activateUserHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		TokenPlainText string `json:"token"`
	}
	err := bknd.readJSON(w, r, &input)
	if err != nil {
		bknd.badRequestResponse(w, r, err)
		return
	}
	vldtr := validator.NewValidator()
	data.ValidateTokenPlainText(vldtr, input.TokenPlainText)

	if !vldtr.Valid() {
		bknd.failedValidationResponse(w, r, vldtr.Errors)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	user, err := bknd.models.Users.GetForToken(ctx, data.ScopeActivation, input.TokenPlainText)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			vldtr.AddError("token", "invalid or expired activation token")
			bknd.failedValidationResponse(w, r, vldtr.Errors)
		default:
			bknd.commonErrs(w, r, err)
		}
		return
	}
	user.Activated = true

	err = bknd.models.Users.UpdateUser(ctx, user)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrEditConflict):
			bknd.editConflictResponse(w, r)
		default:
			bknd.commonErrs(w, r, err)
		}
		return
	}
	err = bknd.models.Tokens.DeleteAllForUser(ctx, data.ScopeActivation, user.Id)
	if err != nil {
		bknd.commonErrs(w, r, err)
		return
	}
	err = bknd.writeJSON(w, http.StatusOK, envelope{"user": user}, nil)
	if err != nil {
		bknd.serverErrorResponse(w, r, err)
	}
}
