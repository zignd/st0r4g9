package store

import (
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"
)

// Object is the metadata record for a stored object. The bytes live on the
// filesystem at StoragePath (relative to the configured data dir).
type Object struct {
	ID          int64             `json:"-"`
	Key         string            `json:"key"`
	Size        int64             `json:"size"`
	ContentType string            `json:"contentType"`
	ETag        string            `json:"etag"`
	StoragePath string            `json:"-"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

// PutObjectInput carries everything needed to create or replace an object.
type PutObjectInput struct {
	BucketID    int64
	Key         string
	Size        int64
	ContentType string
	ETag        string
	StoragePath string
	Metadata    map[string]string
	Annotations map[string]string
}

// PutObject upserts an object within a transaction: it writes the object row
// plus its metadata/annotations. If an object already existed at the same key,
// its previous storage path is returned so the caller can delete the old blob.
func (s *Store) PutObject(in PutObjectInput) (obj Object, oldStoragePath string, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Object{}, "", err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	now := time.Now().UTC()
	nowStr := now.Format(time.RFC3339Nano)

	// Look up any existing object at this key to preserve created_at and to
	// report the old blob path for cleanup.
	var (
		existingID  int64
		oldPath     string
		createdAt   = nowStr
		hasExisting bool
	)
	row := tx.QueryRow(`SELECT id, storage_path, created_at FROM objects WHERE bucket_id = ? AND key = ?`, in.BucketID, in.Key)
	switch scanErr := row.Scan(&existingID, &oldPath, &createdAt); {
	case scanErr == nil:
		hasExisting = true
	case errors.Is(scanErr, sql.ErrNoRows):
		hasExisting = false
	default:
		return Object{}, "", scanErr
	}

	var objID int64
	if hasExisting {
		if _, err = tx.Exec(
			`UPDATE objects SET size = ?, content_type = ?, etag = ?, storage_path = ?, updated_at = ? WHERE id = ?`,
			in.Size, in.ContentType, in.ETag, in.StoragePath, nowStr, existingID,
		); err != nil {
			return Object{}, "", err
		}
		objID = existingID
		oldStoragePath = oldPath
		// Replace child rows.
		if _, err = tx.Exec(`DELETE FROM object_metadata WHERE object_id = ?`, objID); err != nil {
			return Object{}, "", err
		}
		if _, err = tx.Exec(`DELETE FROM object_annotations WHERE object_id = ?`, objID); err != nil {
			return Object{}, "", err
		}
	} else {
		var res sql.Result
		res, err = tx.Exec(
			`INSERT INTO objects (bucket_id, key, size, content_type, etag, storage_path, created_at, updated_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			in.BucketID, in.Key, in.Size, in.ContentType, in.ETag, in.StoragePath, nowStr, nowStr,
		)
		if err != nil {
			return Object{}, "", err
		}
		if objID, err = res.LastInsertId(); err != nil {
			return Object{}, "", err
		}
	}

	if err = insertPairs(tx, `object_metadata`, objID, in.Metadata); err != nil {
		return Object{}, "", err
	}
	if err = insertPairs(tx, `object_annotations`, objID, in.Annotations); err != nil {
		return Object{}, "", err
	}

	if err = tx.Commit(); err != nil {
		return Object{}, "", err
	}

	created, _ := time.Parse(time.RFC3339Nano, createdAt)
	return Object{
		ID:          objID,
		Key:         in.Key,
		Size:        in.Size,
		ContentType: in.ContentType,
		ETag:        in.ETag,
		StoragePath: in.StoragePath,
		CreatedAt:   created,
		UpdatedAt:   now,
		Metadata:    in.Metadata,
		Annotations: in.Annotations,
	}, oldStoragePath, nil
}

func insertPairs(tx *sql.Tx, table string, objID int64, pairs map[string]string) error {
	if len(pairs) == 0 {
		return nil
	}
	// Table name is a constant string literal from trusted call sites, not user input.
	stmt, err := tx.Prepare(`INSERT INTO ` + table + ` (object_id, name, value) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for name, value := range pairs {
		if _, err := stmt.Exec(objID, name, value); err != nil {
			return err
		}
	}
	return nil
}

// GetObject fetches an object's full metadata (including metadata/annotation
// maps) by bucket and key. Returns ErrNotFound when absent.
func (s *Store) GetObject(bucketID int64, key string) (Object, error) {
	var (
		o                    Object
		createdAt, updatedAt string
	)
	err := s.db.QueryRow(
		`SELECT id, key, size, content_type, etag, storage_path, created_at, updated_at
		 FROM objects WHERE bucket_id = ? AND key = ?`,
		bucketID, key,
	).Scan(&o.ID, &o.Key, &o.Size, &o.ContentType, &o.ETag, &o.StoragePath, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Object{}, ErrNotFound
	}
	if err != nil {
		return Object{}, err
	}
	o.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
	o.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)

	if o.Metadata, err = s.loadPairs(`object_metadata`, o.ID); err != nil {
		return Object{}, err
	}
	if o.Annotations, err = s.loadPairs(`object_annotations`, o.ID); err != nil {
		return Object{}, err
	}
	return o, nil
}

func (s *Store) loadPairs(table string, objID int64) (map[string]string, error) {
	rows, err := s.db.Query(`SELECT name, value FROM `+table+` WHERE object_id = ?`, objID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return nil, err
		}
		out[name] = value
	}
	if len(out) == 0 {
		return nil, rows.Err()
	}
	return out, rows.Err()
}

// ListObjectsInput filters a bucket listing.
type ListObjectsInput struct {
	BucketID int64
	Prefix   string
	// Delimiter, when non-empty, folds keys sharing a segment up to the next
	// delimiter into CommonPrefixes (mimicking folders). Typically "/".
	Delimiter string
	// AnnotationFilters restricts results to objects that have every listed
	// annotation name set to the given value.
	AnnotationFilters map[string]string
}

// ListObjectsResult is the outcome of a bucket listing.
type ListObjectsResult struct {
	Objects        []Object
	CommonPrefixes []string
}

// ListObjects lists objects in a bucket applying prefix, annotation, and
// delimiter (folder-folding) semantics. Returned objects carry their metadata
// and annotation maps.
func (s *Store) ListObjects(in ListObjectsInput) (ListObjectsResult, error) {
	query := `SELECT DISTINCT o.id, o.key, o.size, o.content_type, o.etag, o.storage_path, o.created_at, o.updated_at
	          FROM objects o`
	args := []any{}
	where := []string{"o.bucket_id = ?"}
	args = append(args, in.BucketID)

	// Each annotation filter becomes an EXISTS subquery so filters AND together.
	for name, value := range in.AnnotationFilters {
		where = append(where, `EXISTS (SELECT 1 FROM object_annotations a WHERE a.object_id = o.id AND a.name = ? AND a.value = ?)`)
		args = append(args, name, value)
	}
	if in.Prefix != "" {
		where = append(where, `o.key LIKE ? ESCAPE '\'`)
		args = append(args, escapeLike(in.Prefix)+"%")
	}
	query += " WHERE " + strings.Join(where, " AND ") + " ORDER BY o.key"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return ListObjectsResult{}, err
	}
	defer rows.Close()

	result := ListObjectsResult{Objects: []Object{}, CommonPrefixes: []string{}}
	prefixSet := map[string]struct{}{}

	for rows.Next() {
		var (
			o                    Object
			createdAt, updatedAt string
		)
		if err := rows.Scan(&o.ID, &o.Key, &o.Size, &o.ContentType, &o.ETag, &o.StoragePath, &createdAt, &updatedAt); err != nil {
			return ListObjectsResult{}, err
		}
		o.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdAt)
		o.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)

		// Delimiter folding: if the key past the prefix contains the delimiter,
		// surface the common prefix instead of the object itself.
		if in.Delimiter != "" {
			rest := strings.TrimPrefix(o.Key, in.Prefix)
			if idx := strings.Index(rest, in.Delimiter); idx >= 0 {
				prefixSet[in.Prefix+rest[:idx+len(in.Delimiter)]] = struct{}{}
				continue
			}
		}

		if o.Metadata, err = s.loadPairs(`object_metadata`, o.ID); err != nil {
			return ListObjectsResult{}, err
		}
		if o.Annotations, err = s.loadPairs(`object_annotations`, o.ID); err != nil {
			return ListObjectsResult{}, err
		}
		result.Objects = append(result.Objects, o)
	}
	if err := rows.Err(); err != nil {
		return ListObjectsResult{}, err
	}

	for p := range prefixSet {
		result.CommonPrefixes = append(result.CommonPrefixes, p)
	}
	sort.Strings(result.CommonPrefixes)
	return result, nil
}

// DeleteObject removes an object row and returns its storage path so the caller
// can delete the blob. Returns ErrNotFound when absent.
func (s *Store) DeleteObject(bucketID int64, key string) (storagePath string, err error) {
	err = s.db.QueryRow(`SELECT storage_path FROM objects WHERE bucket_id = ? AND key = ?`, bucketID, key).Scan(&storagePath)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	if _, err = s.db.Exec(`DELETE FROM objects WHERE bucket_id = ? AND key = ?`, bucketID, key); err != nil {
		return "", err
	}
	return storagePath, nil
}

// StoragePaths returns the blob paths of every object in a bucket — used to
// clean up the filesystem when force-deleting a non-empty bucket.
func (s *Store) StoragePaths(bucketID int64) ([]string, error) {
	rows, err := s.db.Query(`SELECT storage_path FROM objects WHERE bucket_id = ?`, bucketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	paths := []string{}
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

// escapeLike escapes LIKE wildcards so a user-supplied prefix is matched literally.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
