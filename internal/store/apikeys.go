package store

import (
	"database/sql"
	"errors"
	"time"
)

// APIKey is a stored API credential. The raw secret is never persisted — only
// its SHA-256 hash (KeyHash) and a display-safe prefix.
type APIKey struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	KeyPrefix string    `json:"keyPrefix"`
	CreatedAt time.Time `json:"createdAt"`
}

// CreateAPIKey inserts a new API key given its name, hash, and display prefix.
func (s *Store) CreateAPIKey(name, keyHash, keyPrefix string) (APIKey, error) {
	now := time.Now().UTC()
	res, err := s.db.Exec(
		`INSERT INTO api_keys (name, key_hash, key_prefix, created_at) VALUES (?, ?, ?, ?)`,
		name, keyHash, keyPrefix, now.Format(time.RFC3339Nano),
	)
	if err != nil {
		return APIKey{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return APIKey{}, err
	}
	return APIKey{ID: id, Name: name, KeyPrefix: keyPrefix, CreatedAt: now}, nil
}

// APIKeyByHash looks up an API key by the SHA-256 hash of its raw secret.
// Returns ErrNotFound when no key matches.
func (s *Store) APIKeyByHash(keyHash string) (APIKey, error) {
	var (
		k         APIKey
		createdAt string
	)
	err := s.db.QueryRow(
		`SELECT id, name, key_prefix, created_at FROM api_keys WHERE key_hash = ?`,
		keyHash,
	).Scan(&k.ID, &k.Name, &k.KeyPrefix, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return APIKey{}, ErrNotFound
	}
	if err != nil {
		return APIKey{}, err
	}
	k.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return k, nil
}
