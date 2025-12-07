package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"FernArchive/internal/validator"
)

type Movie struct {
	Id        int64     `json:"id"`
	CreatedAt time.Time `json:"-"`
	Title     string    `json:"title"`
	Runtime   Runtime   `json:"runtime,omitempty"`
	Year      int32     `json:"year,omitempty"`
	Genres    []string  `json:"genres,omitempty"`
}

type MovieModel struct {
	DB *sql.DB
}

func (mdl *MovieModel) Insert(ctx context.Context, movie *Movie) error {
	if len(movie.Genres) < 1 {
		return errors.New("genres must not be empty")
	}
	errFn := func(err error) error {
		return fmt.Errorf("InsertMovie failed: %v", err)
	}
	tx, err := mdl.DB.BeginTx(ctx, nil)
	if err != nil {
		return errFn(err)
	}
	defer func() { _ = tx.Rollback() }()

	placeholders := strings.Repeat("?,", len(movie.Genres))
	placeholders = placeholders[:len(placeholders)-1]
	var (
		query = fmt.Sprintf(`SELECT id FROM genres WHERE genres.name IN (%s)`, placeholders)
		args  = make([]any, 0, len(movie.Genres))
	)
	for _, genre := range movie.Genres {
		args = append(args, genre)
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return errFn(err)
	}
	defer func(rows *sql.Rows) {
		if err := rows.Close(); err != nil {
			slog.Error("Failed to close rows: ", "err", err)
		}
	}(rows)
	var genreIds []int64

	for rows.Next() {
		var gid int64
		if err := rows.Scan(&gid); err != nil {
			return errFn(err)
		}
		genreIds = append(genreIds, gid)
	}
	if err = rows.Err(); err != nil {
		return errFn(err)
	}
	if len(genreIds) != len(movie.Genres) {
		return errFn(fmt.Errorf("genres mismatch"))
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO movies (title, year, runtime) VALUES (?, ?, ?)`,
		movie.Title, movie.Year, movie.Runtime,
	)
	if err != nil {
		return errFn(err)
	}
	movie.Id, err = res.LastInsertId()
	if err != nil {
		return errFn(err)
	}
	for _, gid := range genreIds {
		_, err := tx.ExecContext(ctx, `INSERT INTO movie_genres (movie_id, genre_id) VALUES (?, ?)`,
			movie.Id, gid,
		)
		if err != nil {
			return errFn(err)
		}
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	return nil
}

func (mdl *MovieModel) Get(ctx context.Context, movieId int64) (*Movie, error) {
	if movieId < 1 {
		return nil, ErrRecordNotFound
	}
	errFn := func(err error) error {
		return fmt.Errorf("GetMovie failed: %v", err)
	}
	query := `SELECT m.id, m.created_at, m.title, m.year, m.runtime, GROUP_CONCAT(g.name)
		    FROM movies AS m 
		    LEFT JOIN movie_genres AS mg ON mg.movie_id = m.id
                LEFT JOIN genres AS g ON g.id = mg.genre_id WHERE m.id = ? GROUP BY m.id`

	row := mdl.DB.QueryRowContext(ctx, query, movieId)

	var m Movie
	var genres string

	err := row.Scan(&m.Id, &m.CreatedAt, &m.Title, &m.Year, &m.Runtime, &genres)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, errFn(err)
		}
	}
	if genres != "" {
		m.Genres = strings.Split(genres, ",")
	}
	return &m, nil
}

func (mdl *MovieModel) GetAllByTitle(ctx context.Context, title string, ftr Filters) ([]*Movie, Metadata, error) {
	query := fmt.Sprintf(
		`SELECT m.id, m.created_at, m.title, m.year, m.runtime, GROUP_CONCAT(DISTINCT g.name) 
			  FROM movies AS m 
			  LEFT JOIN movie_genres AS mg ON mg.movie_id = m.id
            	  LEFT JOIN genres AS g ON g.id = mg.genre_id 
			  WHERE LOWER(m.title) LIKE LOWER(CONCAT('%', ?, '%'))
			  GROUP BY m.id ORDER BY %s %s, id ASC LIMIT ? OFFSET ?`, ftr.sortParam(), ftr.sortOrder(),
	)
	args := []any{title, ftr.limit(), ftr.offset()}

	rows, err := mdl.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, Metadata{}, err
	}
	defer func(rows *sql.Rows) {
		if err := rows.Close(); err != nil {
			slog.Error("Failed to close rows: ", "err", err)
		}
	}(rows)
	var (
		movies   []*Movie
		genres   string
		metadata Metadata
	)
	for rows.Next() {
		var m Movie
		err := rows.Scan(&m.Id, &m.CreatedAt, &m.Title, &m.Year, &m.Runtime, &genres)
		if err != nil {
			return nil, metadata, err
		}
		if genres != "" {
			m.Genres = strings.Split(genres, ",")
		}
		movies = append(movies, &m)
	}
	if err = rows.Err(); err != nil {
		return nil, metadata, err
	}
	metadata = calculateMetadata(len(movies), ftr.Page, ftr.PageSize)
	return movies, metadata, nil
}

func (mdl *MovieModel) Update(ctx context.Context, movie *Movie) error {
	errFn := func(err error) error {
		return fmt.Errorf("UpdateMovie failed: %v", err)
	}
	tx, err := mdl.DB.BeginTx(ctx, nil)
	if err != nil {
		return errFn(err)
	}
	defer func() { _ = tx.Rollback() }()
	var (
		query = `UPDATE movies SET title=?, runtime=?, year=? WHERE id = ?`
		args  = []any{movie.Title, movie.Runtime, movie.Year, movie.Id}
	)
	res, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return errFn(err)
	}
	row, err := res.RowsAffected()
	if err != nil {
		return errFn(err)
	}
	if row != 1 {
		return ErrEditConflict
	}
	genreMap := make(map[string]int64)

	rows, err := tx.QueryContext(ctx, `SELECT id, name FROM genres`)
	if err != nil {
		return errFn(err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			slog.Error("Failed to close rows: ", "err", err)
		}
	}()
	for rows.Next() {
		var (
			genre_id int64
			name     string
		)
		if err := rows.Scan(&genre_id, &name); err != nil {
			return errFn(err)
		}
		genreMap[name] = genre_id
	}
	if err = rows.Err(); err != nil {
		return errFn(err)
	}
	query = `DELETE FROM movie_genres WHERE movie_id = ?`

	_, err = tx.ExecContext(ctx, query, movie.Id)
	if err != nil {
		return errFn(err)
	}
	query = `INSERT INTO movie_genres (movie_id, genre_id) VALUES (?, ?)`

	for _, genre := range movie.Genres {
		genre_id, ok := genreMap[genre]
		if !ok {
			return errFn(fmt.Errorf("unknown genre %s", genre))
		}
		_, err = tx.ExecContext(ctx, query, movie.Id, genre_id)
		if err != nil {
			return errFn(err)
		}
	}
	if err = tx.Commit(); err != nil {
		return errFn(fmt.Errorf("failed to commit transaction: %w", err))
	}
	return nil
}

func (mdl *MovieModel) Delete(ctx context.Context, id int64) error {
	if id < 1 {
		return ErrRecordNotFound
	}
	result, err := mdl.DB.ExecContext(ctx, `DELETE FROM movies WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
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
	vldtr.Check(len(movie.Title) <= 50, "title", "must not be more than 500 bytes long")

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
