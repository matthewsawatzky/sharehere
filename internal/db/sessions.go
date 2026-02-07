package db

import (
	"database/sql"
	"fmt"
	"time"
)

func (s *Store) CreateSession(sess Session) error {
	remember := 0
	if sess.Remember {
		remember = 1
	}
	_, err := s.db.Exec(`INSERT INTO sessions(token, user_id, csrf_token, remember, ip, user_agent, expires_at, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		sess.Token, sess.UserID, sess.CSRFToken, remember, sess.IP, sess.UserAgent, sess.ExpiresAt)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (s *Store) GetSession(token string) (Session, error) {
	var sess Session
	var remember int
	err := s.db.QueryRow(`SELECT token, user_id, csrf_token, remember, ip, user_agent, expires_at, created_at, last_seen_at
		FROM sessions WHERE token = ?`, token).
		Scan(&sess.Token, &sess.UserID, &sess.CSRFToken, &remember, &sess.IP, &sess.UserAgent, &sess.ExpiresAt, &sess.CreatedAt, &sess.LastSeenAt)
	if err != nil {
		return Session{}, err
	}
	sess.Remember = remember == 1
	if time.Now().After(sess.ExpiresAt) {
		_ = s.DeleteSession(token)
		return Session{}, sql.ErrNoRows
	}
	return sess, nil
}

func (s *Store) TouchSession(token string, expiresAt time.Time) error {
	_, err := s.db.Exec(`UPDATE sessions SET expires_at = ?, last_seen_at = CURRENT_TIMESTAMP WHERE token = ?`, expiresAt, token)
	if err != nil {
		return fmt.Errorf("touch session: %w", err)
	}
	return nil
}

func (s *Store) RotateSession(oldToken string, newSession Session) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM sessions WHERE token = ?`, oldToken); err != nil {
		return err
	}
	remember := 0
	if newSession.Remember {
		remember = 1
	}
	if _, err := tx.Exec(`INSERT INTO sessions(token, user_id, csrf_token, remember, ip, user_agent, expires_at, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		newSession.Token, newSession.UserID, newSession.CSRFToken, remember, newSession.IP, newSession.UserAgent, newSession.ExpiresAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s *Store) PurgeExpiredSessions() error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE expires_at < CURRENT_TIMESTAMP`)
	if err != nil {
		return fmt.Errorf("purge sessions: %w", err)
	}
	return nil
}
