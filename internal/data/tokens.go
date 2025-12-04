package data

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"time"

	"FernArchive/internal/validator"
)

const (
	ScopeActivation     = "activation"
	ScopeAuthentication = "authentication"
)

type Token struct {
	PlainText string    `json:"token"`
	Hash      []byte    `json:"-"`
	UserId    string    `json:"-"`
	Expiry    time.Time `json:"expiry"`
	Scope     string    `json:"-"`
}

func generateToken(userId string, ttl time.Duration, scope string) (*Token, error) {
	token := &Token{
		UserId: userId,
		Scope:  scope,
		Expiry: time.Now().Add(ttl),
	}
	randomBytes := make([]byte, 16)
	_, _ = rand.Read(randomBytes)

	token.PlainText = base32.StdEncoding.
		WithPadding(base32.NoPadding).EncodeToString(randomBytes)

	hash := sha256.Sum256([]byte(token.PlainText))
	token.Hash = hash[:]
	return token, nil
}

func ValidateTokenPlainText(vldtr *validator.Validator, plainTxt string) {
	vldtr.Check(plainTxt != "", "plainTxt", "must be provided")
	vldtr.Check(len(plainTxt) == 26, "plainTxt", "must be 26 characters long")
}

type TokenModel struct {
	Db *sql.DB
}

func (mdl *TokenModel) NewToken(ctx context.Context, userId string, ttl time.Duration, scope string) (*Token, error) {
	token, err := generateToken(userId, ttl, scope)
	if err != nil {
		return nil, err
	}
	err = mdl.Insert(ctx, token)
	return token, err
}

func (mdl *TokenModel) Insert(ctx context.Context, token *Token) error {
	var (
		query = `INSERT INTO tokens (hash, user_id, expiry, scope) VALUES (?, ?, ?, ?)`
		args  = []any{token.Hash, token.UserId, token.Expiry, token.Scope}
	)
	_, err := mdl.Db.ExecContext(ctx, query, args...)
	return err
}

func (mdl *TokenModel) DeleteAllForUser(ctx context.Context, scope string, userId string) error {
	var (
		query = `DELETE FROM tokens WHERE scope=? AND user_id=?`
		args  = []any{scope, userId}
	)
	_, err := mdl.Db.ExecContext(ctx, query, args...)
	return err
}
