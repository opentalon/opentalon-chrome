package store

import (
	"database/sql"
	"fmt"
	"time"
)

// Store provides CRUD access to browser credentials.
type Store struct {
	db *DB
}

// New returns a Store backed by the given DB.
func New(db *DB) *Store {
	return &Store{db: db}
}

// Save inserts or replaces the cookie JSON for (entityID, name).
func (s *Store) Save(entityID, name, cookiesJSON string) error {
	if entityID == "" {
		return fmt.Errorf("store: entity_id is required")
	}
	if name == "" {
		return fmt.Errorf("store: name is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.SQLDB().Exec(
		`INSERT INTO browser_credentials (entity_id, name, cookies, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(entity_id, name) DO UPDATE SET cookies=excluded.cookies, updated_at=excluded.updated_at`,
		entityID, name, cookiesJSON, now, now,
	)
	return err
}

// Get returns the cookie JSON stored under (entityID, name).
// Returns an error if no record exists.
func (s *Store) Get(entityID, name string) (string, error) {
	var cookies string
	err := s.db.SQLDB().QueryRow(
		`SELECT cookies FROM browser_credentials WHERE entity_id = ? AND name = ?`,
		entityID, name,
	).Scan(&cookies)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("no credentials found for %q", name)
	}
	return cookies, err
}

// List returns the names of all saved credentials for entityID.
func (s *Store) List(entityID string) ([]string, error) {
	rows, err := s.db.SQLDB().Query(
		`SELECT name FROM browser_credentials WHERE entity_id = ? ORDER BY name`,
		entityID,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		names = append(names, n)
	}
	return names, rows.Err()
}

// Delete removes the credential record for (entityID, name).
func (s *Store) Delete(entityID, name string) error {
	_, err := s.db.SQLDB().Exec(
		`DELETE FROM browser_credentials WHERE entity_id = ? AND name = ?`,
		entityID, name,
	)
	return err
}
