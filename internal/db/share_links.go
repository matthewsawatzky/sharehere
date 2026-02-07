package db

import (
	"fmt"
	"time"
)

func (s *Store) CreateShareLink(link ShareLink) error {
	_, err := s.db.Exec(`INSERT INTO share_links(token, path, mode, created_by, expires_at, revoked, created_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		link.Token, link.Path, link.Mode, link.CreatedBy, link.ExpiresAt, boolToInt(link.Revoked))
	if err != nil {
		return fmt.Errorf("create share link: %w", err)
	}
	return nil
}

func (s *Store) GetShareLink(token string) (ShareLink, error) {
	var link ShareLink
	var revoked int
	var last sqlNullTime
	err := s.db.QueryRow(`SELECT token, path, mode, created_by, expires_at, revoked, created_at, last_accessed_at
		FROM share_links WHERE token = ?`, token).
		Scan(&link.Token, &link.Path, &link.Mode, &link.CreatedBy, &link.ExpiresAt, &revoked, &link.CreatedAt, &last)
	if err != nil {
		return ShareLink{}, err
	}
	link.Revoked = revoked == 1
	if last.Valid {
		t := last.Time
		link.LastAccessed = &t
	}
	return link, nil
}

func (s *Store) MarkShareLinkAccessed(token string) error {
	_, err := s.db.Exec(`UPDATE share_links SET last_accessed_at = CURRENT_TIMESTAMP WHERE token = ?`, token)
	if err != nil {
		return fmt.Errorf("touch share link: %w", err)
	}
	return nil
}

func (s *Store) RevokeShareLink(token string) error {
	_, err := s.db.Exec(`UPDATE share_links SET revoked = 1 WHERE token = ?`, token)
	if err != nil {
		return fmt.Errorf("revoke share link: %w", err)
	}
	return nil
}

func (s *Store) ListShareLinks() ([]ShareLink, error) {
	rows, err := s.db.Query(`SELECT token, path, mode, created_by, expires_at, revoked, created_at, last_accessed_at
		FROM share_links ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list share links: %w", err)
	}
	defer rows.Close()

	items := make([]ShareLink, 0)
	for rows.Next() {
		var l ShareLink
		var revoked int
		var last sqlNullTime
		if err := rows.Scan(&l.Token, &l.Path, &l.Mode, &l.CreatedBy, &l.ExpiresAt, &revoked, &l.CreatedAt, &last); err != nil {
			return nil, err
		}
		l.Revoked = revoked == 1
		if last.Valid {
			t := last.Time
			l.LastAccessed = &t
		}
		items = append(items, l)
	}
	return items, rows.Err()
}

type sqlNullTime struct {
	Time  time.Time
	Valid bool
}

func (nt *sqlNullTime) Scan(value any) error {
	if value == nil {
		nt.Time, nt.Valid = time.Time{}, false
		return nil
	}
	switch v := value.(type) {
	case time.Time:
		nt.Time, nt.Valid = v, true
		return nil
	case string:
		if v == "" {
			nt.Time, nt.Valid = time.Time{}, false
			return nil
		}
		t, err := time.Parse("2006-01-02 15:04:05", v)
		if err != nil {
			t, err = time.Parse(time.RFC3339Nano, v)
			if err != nil {
				return err
			}
		}
		nt.Time, nt.Valid = t, true
		return nil
	case []byte:
		return nt.Scan(string(v))
	default:
		return fmt.Errorf("unsupported Scan value for sqlNullTime: %T", value)
	}
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
