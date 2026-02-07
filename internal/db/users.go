package db

import (
	"database/sql"
	"fmt"
	"strings"
)

func (s *Store) CreateUser(username, passwordHash, role string) (int64, error) {
	username = strings.TrimSpace(strings.ToLower(username))
	res, err := s.db.Exec(`INSERT INTO users(username, password_hash, role) VALUES (?, ?, ?)`, username, passwordHash, role)
	if err != nil {
		return 0, fmt.Errorf("create user: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("user id: %w", err)
	}
	return id, nil
}

func (s *Store) GetUserByUsername(username string) (User, error) {
	username = strings.TrimSpace(strings.ToLower(username))
	var u User
	var disabled int
	err := s.db.QueryRow(`SELECT id, username, password_hash, role, disabled, created_at, updated_at FROM users WHERE username = ?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &disabled, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return User{}, err
	}
	u.Disabled = disabled == 1
	return u, nil
}

func (s *Store) GetUserByID(id int64) (User, error) {
	var u User
	var disabled int
	err := s.db.QueryRow(`SELECT id, username, password_hash, role, disabled, created_at, updated_at FROM users WHERE id = ?`, id).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &disabled, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return User{}, err
	}
	u.Disabled = disabled == 1
	return u, nil
}

func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`SELECT id, username, password_hash, role, disabled, created_at, updated_at FROM users ORDER BY username ASC`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	out := make([]User, 0)
	for rows.Next() {
		var u User
		var disabled int
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &disabled, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		u.Disabled = disabled == 1
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) DeleteUser(username string) error {
	res, err := s.db.Exec(`DELETE FROM users WHERE username = ?`, strings.TrimSpace(strings.ToLower(username)))
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SetUserPassword(username, passwordHash string) error {
	res, err := s.db.Exec(`UPDATE users SET password_hash = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, passwordHash, strings.TrimSpace(strings.ToLower(username)))
	if err != nil {
		return fmt.Errorf("set password: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SetUserDisabled(username string, disabled bool) error {
	v := 0
	if disabled {
		v = 1
	}
	res, err := s.db.Exec(`UPDATE users SET disabled = ?, updated_at = CURRENT_TIMESTAMP WHERE username = ?`, v, strings.TrimSpace(strings.ToLower(username)))
	if err != nil {
		return fmt.Errorf("set disabled: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) AdminCount() (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM users WHERE role = 'admin' AND disabled = 0`).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}
