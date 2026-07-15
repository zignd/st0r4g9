package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Bucket is a logical container owned by a single API key.
type Bucket struct {
	ID        int64     `json:"-"`
	Name      string    `json:"name"`
	Region    string    `json:"region"`
	CreatedAt time.Time `json:"createdAt"`
}

// CreateBucket creates a bucket named name owned by apiKeyID. Returns
// ErrConflict if the caller already owns a bucket with that name.
func (s *Store) CreateBucket(apiKeyID int64, name string) (Bucket, error) {
	now := time.Now().UTC()
	const region = "local-1"
	res, err := s.db.Exec(
		`INSERT INTO buckets (api_key_id, name, region, created_at) VALUES (?, ?, ?, ?)`,
		apiKeyID, name, region, now.Format(time.RFC3339Nano),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Bucket{}, ErrConflict
		}
		return Bucket{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Bucket{}, err
	}
	return Bucket{ID: id, Name: name, Region: region, CreatedAt: now}, nil
}

// BucketByName fetches a bucket by name within the caller's namespace.
// Returns ErrNotFound when the caller owns no such bucket.
func (s *Store) BucketByName(apiKeyID int64, name string) (Bucket, error) {
	var (
		b         Bucket
		createdAt string
	)
	err := s.db.QueryRow(
		`SELECT id, name, region, created_at FROM buckets WHERE api_key_id = ? AND name = ?`,
		apiKeyID, name,
	).Scan(&b.ID, &b.Name, &b.Region, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Bucket{}, ErrNotFound
	}
	if err != nil {
		return Bucket{}, err
	}
	b.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	return b, nil
}

// ListBuckets returns all buckets owned by the caller, ordered by name.
func (s *Store) ListBuckets(apiKeyID int64) ([]Bucket, error) {
	rows, err := s.db.Query(
		`SELECT id, name, region, created_at FROM buckets WHERE api_key_id = ? ORDER BY name`,
		apiKeyID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	buckets := []Bucket{}
	for rows.Next() {
		var (
			b         Bucket
			createdAt string
		)
		if err := rows.Scan(&b.ID, &b.Name, &b.Region, &createdAt); err != nil {
			return nil, err
		}
		b.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		buckets = append(buckets, b)
	}
	return buckets, rows.Err()
}

// CountObjects returns how many objects the bucket currently holds.
func (s *Store) CountObjects(bucketID int64) (int, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM objects WHERE bucket_id = ?`, bucketID).Scan(&n)
	return n, err
}

// DeleteBucket removes a bucket (and, via cascade, its object rows). Blob files
// must be removed separately by the caller. Returns ErrNotFound if absent.
func (s *Store) DeleteBucket(bucketID int64) error {
	res, err := s.db.Exec(`DELETE FROM buckets WHERE id = ?`, bucketID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	// modernc.org/sqlite surfaces constraint errors in the message text.
	return err != nil && strings.Contains(strings.ToUpper(err.Error()), "UNIQUE")
}
