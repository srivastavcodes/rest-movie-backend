package data

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"
)

type Permissions []string

func (p Permissions) Include(code string) bool {
	return slices.Contains(p, code)
}

type PermissionModel struct {
	Db *sql.DB
}

func (mdl PermissionModel) GetAllForUser(ctx context.Context, userId int64) (Permissions, error) {
	query := `SELECT p.code FROM permissions AS p INNER JOIN user_permissions AS up ON up.permission_id = p.id
                INNER JOIN users AS u ON up.user_id = u.id WHERE u.id = ?`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rows, err := mdl.Db.QueryContext(ctx, query, userId)
	if err != nil {
		return nil, err
	}
	defer func(rows *sql.Rows) {
		if err := rows.Close(); err != nil {
			slog.Error("Failed to close rows: ", "err", err)
		}
	}(rows)
	var permissions Permissions

	for rows.Next() {
		var permission string
		if err := rows.Scan(&permission); err != nil {
			return nil, err
		}
		permissions = append(permissions, permission)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return permissions, nil
}

func (mdl PermissionModel) AddForUser(ctx context.Context, userId int64, codes ...string) error {
	if len(codes) <= 0 {
		return nil
	}
	placeholders := strings.Repeat("?,", len(codes))
	placeholders = placeholders[:len(placeholders)-1]

	query := fmt.Sprintf(`INSERT INTO user_permissions SELECT ?, p.id FROM permissions AS p WHERE p.code IN (%s)`,
		placeholders,
	)
	args := make([]any, 0, len(codes)+1)
	args = append(args, userId)

	for _, code := range codes {
		args = append(args, code)
	}
	_, err := mdl.Db.ExecContext(ctx, query, args...)
	return err
}
