package data

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"time"

	"FernArchive/internal/validator"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var ErrDuplicateEmail = errors.New("duplicate email")

var AnonymousUser = &User{}

type User struct {
	Id        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Password  password  `json:"-"`
	Activated bool      `json:"activated"`
}

func (usr *User) IsAnonymous() bool {
	return usr == AnonymousUser
}

type password struct {
	plainText *string
	hash      []byte
}

func (pass *password) SetPass(plainTxtPass string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(plainTxtPass), 12)
	if err != nil {
		return err
	}
	pass.plainText = &plainTxtPass
	pass.hash = hash
	return nil
}

func (pass *password) CheckPass(plainTxtPass string) (bool, error) {
	err := bcrypt.CompareHashAndPassword(pass.hash, []byte(plainTxtPass))
	if err != nil {
		switch {
		case errors.Is(err, bcrypt.ErrMismatchedHashAndPassword):
			return false, nil
		default:
			return false, err
		}
	}
	return true, nil
}

func ValidateEmail(vldtr *validator.Validator, email string) {
	vldtr.Check(email != "", "email", "must be provided")
	vldtr.Check(validator.Matches(email, validator.EmailRX), "email", "invalid email address")
}

func ValidatePasswordPlainTxt(vldtr *validator.Validator, pass string) {
	vldtr.Check(pass != "", "password", "must be provided")
	vldtr.Check(len(pass) >= 8, "password", "must be greater than 8 chars")
	vldtr.Check(len(pass) <= 72, "password", "must be lesser than 72 chars")
}

func ValidateUser(vldtr *validator.Validator, user *User) {
	vldtr.Check(user.Name != "", "name", "must be provided")
	vldtr.Check(len(user.Name) <= 500, "name", "must be less than 500 chars")

	ValidateEmail(vldtr, user.Email)

	if user.Password.plainText != nil {
		ValidatePasswordPlainTxt(vldtr, *user.Password.plainText)
	}
	if user.Password.hash == nil {
		panic("missing password hash for user")
	}
}

type UserModel struct {
	Db *sql.DB
}

func (mdl *UserModel) InsertUser(user *User, ctx context.Context) error {
	user.Id = uuid.New().String()
	var (
		query = `INSERT INTO users (id, name, email, password_hash, activated)  VALUES (?, ?, ?, ?, ?)`
		args  = []any{user.Id, user.Name, user.Email, user.Password.hash, user.Activated}
	)
	_, err := mdl.Db.ExecContext(ctx, query, args...)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		switch {
		case errors.As(err, &mysqlErr) && mysqlErr.Number == 1062:
			return ErrDuplicateEmail
		default:
			return err
		}
	}
	return nil
}

func (mdl *UserModel) GetByEmail(email string, ctx context.Context) (*User, error) {
	query := `SELECT id, created_at, name, email, password_hash, activated FROM users WHERE email = ?`
	var user User

	err := mdl.Db.QueryRowContext(ctx, query, email).Scan(&user.Id,
		&user.CreatedAt,
		&user.Name,
		&user.Email,
		&user.Password.hash,
		&user.Activated,
	)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}
	return &user, nil
}

func (mdl *UserModel) UpdateUser(user *User, ctx context.Context) error {
	var (
		query = `UPDATE users SET name=?, email=?, password_hash=?, activated=? WHERE id = ?`
		args  = []any{user.Name, user.Email, user.Password.hash, user.Activated, user.Id}
	)
	res, err := mdl.Db.ExecContext(ctx, query, args...)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		switch {
		case errors.As(err, &mysqlErr) && mysqlErr.Number == 1062:
			return ErrDuplicateEmail
		default:
			return err
		}
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrRecordNotFound
	}
	return nil
}

func (mdl *UserModel) GetForToken(scope, plainTxt string, ctx context.Context) (*User, error) {
	var (
		hash  = sha256.Sum256([]byte(plainTxt))
		query = `SELECT u.id, u.created_at, u.name, u.email, u.password_hash, u.activated FROM users AS u 
		    	   INNER JOIN tokens AS t ON u.id = t.user_id WHERE t.hash = ? AND t.scope = ? 
			   AND t.expiry > ?`

		args = []any{hash[:], scope, time.Now()}
	)
	var user User
	err := mdl.Db.QueryRowContext(ctx, query, args...).Scan(&user.Id,
		&user.CreatedAt,
		&user.Name,
		&user.Email,
		&user.Password.hash,
		&user.Activated,
	)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			return nil, ErrRecordNotFound
		default:
			return nil, err
		}
	}
	return &user, nil
}
