package db

import "fmt"

func (s *Store) RecordAudit(actorUserID *int64, action, target, metadata string) error {
	_, err := s.db.Exec(`INSERT INTO audit_logs(actor_user_id, action, target, metadata) VALUES (?, ?, ?, ?)`, actorUserID, action, target, metadata)
	if err != nil {
		return fmt.Errorf("insert audit: %w", err)
	}
	return nil
}

func (s *Store) ListAudit(limit int) ([]AuditLog, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`SELECT a.id, a.actor_user_id, a.action, a.target, a.metadata, a.created_at, u.username
		FROM audit_logs a
		LEFT JOIN users u ON u.id = a.actor_user_id
		ORDER BY a.created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit: %w", err)
	}
	defer rows.Close()
	logs := make([]AuditLog, 0, limit)
	for rows.Next() {
		var l AuditLog
		if err := rows.Scan(&l.ID, &l.ActorUserID, &l.Action, &l.Target, &l.Metadata, &l.CreatedAt, &l.Username); err != nil {
			return nil, err
		}
		logs = append(logs, l)
	}
	return logs, rows.Err()
}
