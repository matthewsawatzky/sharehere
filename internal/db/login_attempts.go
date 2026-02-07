package db

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"time"
)

func (s *Store) CheckLoginAllowed(key string) (locked bool, retryAfter time.Duration, err error) {
	var ignored int
	var lockedUntil sqlNullTime
	err = s.db.QueryRow(`SELECT failed_count, locked_until FROM login_attempts WHERE key = ?`, key).Scan(&ignored, &lockedUntil)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, 0, nil
		}
		return false, 0, err
	}
	if !lockedUntil.Valid {
		return false, 0, nil
	}
	now := time.Now()
	if lockedUntil.Time.After(now) {
		return true, time.Until(lockedUntil.Time), nil
	}
	return false, 0, nil
}

func (s *Store) RegisterFailedLogin(key string) (time.Duration, error) {
	now := time.Now()
	var failed int
	var locked sqlNullTime
	err := s.db.QueryRow(`SELECT failed_count, locked_until FROM login_attempts WHERE key = ?`, key).Scan(&failed, &locked)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		failed = 0
	}
	failed++
	var lockDuration time.Duration
	if failed >= 5 {
		power := math.Min(float64(failed-5), 5)
		lockDuration = time.Duration(math.Pow(2, power)) * time.Minute
	}
	var lockedUntil any = nil
	if lockDuration > 0 {
		lockedUntil = now.Add(lockDuration)
	}
	_, err = s.db.Exec(`INSERT INTO login_attempts(key, failed_count, locked_until, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(key) DO UPDATE SET failed_count = excluded.failed_count, locked_until = excluded.locked_until, updated_at = CURRENT_TIMESTAMP`,
		key, failed, lockedUntil)
	if err != nil {
		return 0, fmt.Errorf("register failed login: %w", err)
	}
	return lockDuration, nil
}

func (s *Store) ResetLoginAttempts(key string) error {
	_, err := s.db.Exec(`DELETE FROM login_attempts WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("reset login attempts: %w", err)
	}
	return nil
}
