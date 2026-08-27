package storage

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type ModelState struct {
	ModelID     string
	ProviderID  string
	Name        string
	IsFree      bool
	Available   bool // currently listed as free
	CheckedAt   time.Time
	LastChanged time.Time
}

type HistoryPoint struct {
	ModelID   string    `json:"model_id"`
	Provider  string    `json:"provider"`
	Available bool      `json:"available"`
	CheckedAt time.Time `json:"checked_at"`
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on", path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS models (
			model_id    TEXT PRIMARY KEY,
			provider_id TEXT NOT NULL,
			name        TEXT NOT NULL,
			is_free     INTEGER NOT NULL,
			first_seen  INTEGER NOT NULL,
			last_seen   INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS checks (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			model_id    TEXT NOT NULL,
			available   INTEGER NOT NULL,
			checked_at  INTEGER NOT NULL,
			FOREIGN KEY(model_id) REFERENCES models(model_id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_checks_model_time ON checks(model_id, checked_at)`,
		`CREATE INDEX IF NOT EXISTS idx_checks_time ON checks(checked_at)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return fmt.Errorf("migrate: %w (%s)", err, q)
		}
	}
	return nil
}

func (s *Store) UpsertModel(modelID, providerID, name string, isFree bool) error {
	now := time.Now().Unix()
	_, err := s.db.Exec(`
		INSERT INTO models (model_id, provider_id, name, is_free, first_seen, last_seen)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(model_id) DO UPDATE SET
			provider_id = excluded.provider_id,
			name        = excluded.name,
			is_free     = excluded.is_free,
			last_seen   = excluded.last_seen
	`, modelID, providerID, name, boolToInt(isFree), now, now)
	if err != nil {
		return fmt.Errorf("upsert model: %w", err)
	}
	return nil
}

func (s *Store) RecordCheck(modelID string, available bool, at time.Time) error {
	_, err := s.db.Exec(`INSERT INTO checks (model_id, available, checked_at) VALUES (?, ?, ?)`,
		modelID, boolToInt(available), at.Unix())
	if err != nil {
		return fmt.Errorf("insert check: %w", err)
	}
	return nil
}

func (s *Store) AllModels() ([]ModelState, error) {
	rows, err := s.db.Query(`
		SELECT m.model_id, m.provider_id, m.name, m.is_free,
		       COALESCE((SELECT available FROM checks WHERE model_id = m.model_id ORDER BY checked_at DESC LIMIT 1), 0),
		       COALESCE((SELECT checked_at FROM checks WHERE model_id = m.model_id ORDER BY checked_at DESC LIMIT 1), 0)
		FROM models m
		ORDER BY m.is_free DESC, m.provider_id, m.name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ModelState
	for rows.Next() {
		var m ModelState
		var isFree, avail, ts int64
		if err := rows.Scan(&m.ModelID, &m.ProviderID, &m.Name, &isFree, &avail, &ts); err != nil {
			return nil, err
		}
		m.IsFree = isFree != 0
		m.Available = avail != 0
		m.CheckedAt = time.Unix(ts, 0)
		if !m.CheckedAt.IsZero() {
			m.LastChanged = m.CheckedAt
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UptimeFraction returns fraction of checks within [since, now] that were available.
func (s *Store) UptimeFraction(modelID string, since time.Time) (float64, int, error) {
	row := s.db.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN available=1 THEN 1 ELSE 0 END), 0),
			COUNT(*)
		FROM checks
		WHERE model_id = ? AND checked_at >= ?
	`, modelID, since.Unix())
	var ups, total int
	if err := row.Scan(&ups, &total); err != nil {
		return 0, 0, err
	}
	if total == 0 {
		return 0, 0, nil
	}
	return float64(ups) / float64(total), total, nil
}

// History returns check rows for a model within [since, now], ordered ascending.
func (s *Store) History(modelID string, since time.Time) ([]HistoryPoint, error) {
	rows, err := s.db.Query(`
		SELECT model_id, available, checked_at
		FROM checks
		WHERE model_id = ? AND checked_at >= ?
		ORDER BY checked_at ASC
	`, modelID, since.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HistoryPoint
	for rows.Next() {
		var p HistoryPoint
		var avail, ts int64
		if err := rows.Scan(&p.ModelID, &avail, &ts); err != nil {
			return nil, err
		}
		p.Available = avail != 0
		p.CheckedAt = time.Unix(ts, 0)
		// Best-effort provider lookup.
		_ = s.db.QueryRow(`SELECT provider_id FROM models WHERE model_id = ?`, p.ModelID).Scan(&p.Provider)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) Prune(retention time.Duration) (int64, error) {
	cutoff := time.Now().Add(-retention).Unix()
	res, err := s.db.Exec(`DELETE FROM checks WHERE checked_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) Stats() (models int, checks int, err error) {
	row := s.db.QueryRow(`SELECT COUNT(*) FROM models`)
	if err = row.Scan(&models); err != nil {
		return
	}
	row = s.db.QueryRow(`SELECT COUNT(*) FROM checks`)
	err = row.Scan(&checks)
	return
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
