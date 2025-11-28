package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"FernArchive/internal/validator"

	"github.com/lib/pq"
)

type Movie struct {
	Id        int64     `json:"id"`
	CreatedAt time.Time `json:"-"`
	Title     string    `json:"title"`
	Runtime   Runtime   `json:"runtime,omitempty"`
	Year      int32     `json:"year,omitempty"`
	Genres    []string  `json:"genres,omitempty"`
	Version   int32     `json:"version"`
}

type MovieModel struct {
	DB *sql.DB
}

func (mdl *MovieModel) Insert(movie *Movie) error {
	query := `INSERT INTO movies (title, year, runtime, genres)
		    VALUES ($1, $2, $3, $4)
		    RETURNING id, created_at, version`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []any{movie.Title, movie.Year, movie.Runtime, pq.Array(movie.Genres)}
	return mdl.DB.QueryRowContext(ctx, query, args...).Scan(&movie.Id, &movie.CreatedAt, &movie.Version)
}

func (mdl *MovieModel) Get(id int64) (*Movie, error) {
	if id < 1 {
		return nil, ErrRecordNotFound
	}
	query := `SELECT id, created_at, title, year, runtime, genres, version FROM movies
                WHERE id = $1`
	movie := new(Movie)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	err := mdl.DB.QueryRowContext(ctx, query, id).Scan(&movie.Id, &movie.CreatedAt,
		&movie.Title, &movie.Year, &movie.Runtime, pq.Array(&movie.Genres), &movie.Version)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}
	return movie, nil
}

func (mdl *MovieModel) GetAll(title string, genres []string, fltr Filters) ([]*Movie, Metadata, error) {
	query := fmt.Sprintf(`
                    SELECT COUNT(*) OVER(), id, created_at, title, year, runtime, genres, version 
                    FROM movies
                    WHERE (to_tsvector('simple', title) @@ plainto_tsquery('simple', $1) OR $1 = '')
   			  AND (genres @> $2 OR $2 = '{}') 
                    ORDER BY %s %s, id ASC LIMIT $3 OFFSET $4`, fltr.sortParam(), fltr.sortOrder())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []any{title, pq.Array(genres), fltr.limit(), fltr.offset()}
	rows, err := mdl.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, Metadata{}, err
	}
	defer func(rows *sql.Rows) {
		if err := rows.Close(); err != nil {
			slog.Error("Failed to close rows: ", "err", err)
		}
	}(rows)

	var totalRecords = 0
	var movies []*Movie

	for rows.Next() {
		var movie Movie
		err := rows.Scan(&totalRecords, &movie.Id,
			&movie.CreatedAt,
			&movie.Title,
			&movie.Year,
			&movie.Runtime,
			pq.Array(&movie.Genres),
			&movie.Version,
		)
		if err != nil {
			return nil, Metadata{}, err
		}
		movies = append(movies, &movie)
	}
	if err = rows.Err(); err != nil {
		return nil, Metadata{}, err
	}
	metadata := calculateMetadata(totalRecords, fltr.Page, fltr.PageSize)
	return movies, metadata, nil
}

func (mdl *MovieModel) Update(movie *Movie) error {
	query := `UPDATE movies SET title=$1, year=$2, runtime=$3, genres=$4, version=version+1
                WHERE id = $5 AND version=$6 RETURNING version`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	args := []any{movie.Title, movie.Year, movie.Runtime, pq.Array(movie.Genres), movie.Id,
		movie.Version}
	err := mdl.DB.QueryRowContext(ctx, query, args...).Scan(&movie.Version)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return ErrEditConflict
		default:
			return err
		}
	}
	return nil
}

func (mdl *MovieModel) Delete(id int64) error {
	if id < 1 {
		return ErrRecordNotFound
	}
	query := `DELETE FROM movies WHERE id = $1`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := mdl.DB.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrRecordNotFound
	}
	return nil
}

func (movie *Movie) ApplyPartialUpdates(title *string, year *int32, runtime *Runtime, genres []string) {
	if title != nil {
		movie.Title = *title
	}
	if year != nil {
		movie.Year = *year
	}
	if runtime != nil {
		movie.Runtime = *runtime
	}
	if genres != nil {
		movie.Genres = genres
	}
}

func ValidateMovie(vldtr *validator.Validator, movie *Movie) {
	vldtr.Check(movie.Title != "", "title", "must be provided")
	vldtr.Check(len(movie.Title) <= 500, "title", "must not be more than 500 bytes long")

	vldtr.Check(movie.Year != 0, "year", "must be provided")
	vldtr.Check(movie.Year >= 1888, "year", "must be greater than 1888")
	vldtr.Check(movie.Year <= int32(time.Now().Year()), "year", "must not be in the future")

	vldtr.Check(movie.Runtime != 0, "runtime", "must be provided")
	vldtr.Check(movie.Runtime > 0, "runtime", "must be a positive integer")

	vldtr.Check(movie.Genres != nil, "genres", "must be provided")
	vldtr.Check(len(movie.Genres) >= 1, "genres", "must contain at least 1 genre")
	vldtr.Check(len(movie.Genres) <= 5, "genres", "must not contain more than 5 genres")
	vldtr.Check(validator.Unique(movie.Genres), "genres", "must not contain duplicate values")
}
