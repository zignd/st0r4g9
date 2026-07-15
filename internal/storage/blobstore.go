// Package storage stores object bytes on the local filesystem. Object metadata
// lives in package store; this package only moves bytes and reports their
// content hash and size.
package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

// BlobStore writes and reads object blobs under a root directory. Each object
// gets its own file at "<bucketID>/<random-id>"; paths are stored (relative to
// root) in the object row so downloads and deletes can find the bytes.
type BlobStore struct {
	root string
}

// New returns a BlobStore rooted at dir, creating the directory if needed.
func New(dir string) (*BlobStore, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}
	return &BlobStore{root: dir}, nil
}

// WriteResult reports the outcome of streaming a blob to disk.
type WriteResult struct {
	// RelPath is the blob's path relative to the store root; persist this.
	RelPath string
	// ETag is the hex SHA-256 of the content.
	ETag string
	// Size is the number of bytes written.
	Size int64
}

// Write streams r to a temp file (hashing and counting as it goes) for the
// given bucket, then atomically renames it into place. It never buffers the
// whole object in memory, so arbitrarily large uploads are fine.
func (b *BlobStore) Write(bucketID int64, r io.Reader) (WriteResult, error) {
	bucketDir := filepath.Join(b.root, strconv.FormatInt(bucketID, 10))
	if err := os.MkdirAll(bucketDir, 0o755); err != nil {
		return WriteResult{}, err
	}

	tmp, err := os.CreateTemp(bucketDir, ".upload-*")
	if err != nil {
		return WriteResult{}, err
	}
	tmpName := tmp.Name()
	// Best-effort cleanup if we bail before the rename.
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()

	hasher := sha256.New()
	size, err := io.Copy(tmp, io.TeeReader(r, hasher))
	if err != nil {
		tmp.Close()
		return WriteResult{}, err
	}
	if err := tmp.Close(); err != nil {
		return WriteResult{}, err
	}

	id, err := randomID()
	if err != nil {
		return WriteResult{}, err
	}
	relPath := filepath.Join(strconv.FormatInt(bucketID, 10), id)
	finalPath := filepath.Join(b.root, relPath)
	if err := os.Rename(tmpName, finalPath); err != nil {
		return WriteResult{}, err
	}
	committed = true

	return WriteResult{
		RelPath: relPath,
		ETag:    hex.EncodeToString(hasher.Sum(nil)),
		Size:    size,
	}, nil
}

// Open opens a blob for reading (streaming) by its stored relative path.
func (b *BlobStore) Open(relPath string) (*os.File, error) {
	return os.Open(filepath.Join(b.root, relPath))
}

// Delete removes a blob by its relative path. A missing file is not an error.
func (b *BlobStore) Delete(relPath string) error {
	if relPath == "" {
		return nil
	}
	err := os.Remove(filepath.Join(b.root, relPath))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func randomID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
