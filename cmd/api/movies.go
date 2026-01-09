package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"FernArchive/internal/data"
	"FernArchive/internal/validator"
)

func (bknd *backend) createMovieHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title   string       `json:"title"`
		Runtime data.Runtime `json:"runtime"`
		Year    int32        `json:"year"`
		Genres  []string     `json:"genres"`
	}
	err := bknd.readJSON(w, r, &input)
	if err != nil {
		bknd.badRequestResponse(w, r, err)
		return
	}
	vldtr := validator.NewValidator()
	movie := &data.Movie{
		Title:   input.Title,
		Runtime: input.Runtime,
		Year:    input.Year,
		Genres:  input.Genres,
	}
	if data.ValidateMovie(vldtr, movie); !vldtr.Valid() {
		bknd.failedValidationResponse(w, r, vldtr.Errors)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	err = bknd.models.Movies.Insert(ctx, movie)
	if err != nil {
		bknd.serverErrorResponse(w, r, err)
		return
	}
	headers := make(http.Header)
	headers.Set("Location", fmt.Sprintf("/v1/movies/%d", movie.Id))

	err = bknd.writeJSON(w, http.StatusCreated, envelope{"movie": movie}, headers)
	if err != nil {
		bknd.serverErrorResponse(w, r, err)
	}
}

func (bknd *backend) showMovieHandler(w http.ResponseWriter, r *http.Request) {
	id, err := bknd.readIdParam(r)
	if err != nil {
		bknd.notFoundResponse(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	movie, err := bknd.models.Movies.Get(ctx, id)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			bknd.notFoundResponse(w, r)
		default:
			bknd.serverErrorResponse(w, r, err)
		}
		return
	}
	err = bknd.writeJSON(w, http.StatusOK, envelope{"movie": movie}, nil)
	if err != nil {
		bknd.serverErrorResponse(w, r, err)
	}
}

func (bknd *backend) listMovieHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title string
		data.Filters
		Genres []string
	}
	vldtr := validator.NewValidator()
	qs := r.URL.Query()

	input.Title = bknd.readString(qs, "title", "")
	input.Genres = bknd.readCSV(qs, "genres", []string{})

	input.Filters.PageSize = bknd.readInt(qs, "page_size", 20, vldtr)
	input.Filters.Page = bknd.readInt(qs, "page", 1, vldtr)

	input.Filters.Sort = bknd.readString(qs, "sort", "id")
	input.Filters.SortParams = []string{"id", "title", "year", "runtime", "-id", "-title", "-year", "-runtime"}

	if data.ValidateFilters(vldtr, input.Filters); !vldtr.Valid() {
		bknd.failedValidationResponse(w, r, vldtr.Errors)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	movies, metadata, err := bknd.models.Movies.GetAllByTitle(ctx, input.Title, input.Filters)
	if err != nil {
		bknd.serverErrorResponse(w, r, err)
		return
	}
	err = bknd.writeJSON(w, http.StatusOK, envelope{"movies": movies, "metadata": metadata}, nil)
	if err != nil {
		bknd.serverErrorResponse(w, r, err)
	}
}

func (bknd *backend) updateMovieHandler(w http.ResponseWriter, r *http.Request) {
	id, err := bknd.readIdParam(r)
	if err != nil {
		bknd.notFoundResponse(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	movie, err := bknd.models.Movies.Get(ctx, id)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			bknd.notFoundResponse(w, r)
		default:
			bknd.serverErrorResponse(w, r, err)
		}
		return
	}
	var input struct {
		Title   *string       `json:"title"`
		Year    *int32        `json:"year"`
		Runtime *data.Runtime `json:"runtime"`
		Genres  []string      `json:"genres"`
	}
	err = bknd.readJSON(w, r, &input)
	if err != nil {
		bknd.badRequestResponse(w, r, err)
		return
	}
	movie.ApplyPartialUpdates(input.Title, input.Year, input.Runtime, input.Genres)
	vldtr := validator.NewValidator()

	if data.ValidateMovie(vldtr, movie); !vldtr.Valid() {
		bknd.failedValidationResponse(w, r, vldtr.Errors)
		return
	}
	err = bknd.models.Movies.Update(ctx, movie)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrEditConflict):
			bknd.editConflictResponse(w, r)
		default:
			bknd.serverErrorResponse(w, r, err)
		}
		return
	}
	err = bknd.writeJSON(w, http.StatusOK, envelope{"movie": movie}, nil)
	if err != nil {
		bknd.serverErrorResponse(w, r, err)
	}
}

func (bknd *backend) deleteMovieHandler(w http.ResponseWriter, r *http.Request) {
	id, err := bknd.readIdParam(r)
	if err != nil {
		bknd.notFoundResponse(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	err = bknd.models.Movies.Delete(ctx, id)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrRecordNotFound):
			bknd.notFoundResponse(w, r)
		default:
			bknd.serverErrorResponse(w, r, err)
		}
		return
	}
	err = bknd.writeJSON(w, http.StatusOK, envelope{"message": "movie deleted successfully"}, nil)
	if err != nil {
		bknd.serverErrorResponse(w, r, err)
	}
}
