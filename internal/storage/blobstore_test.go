package storage_test

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"strings"
	"testing"

	"github.com/zignd/st0r4g9/internal/storage"
)

func TestWriteReadDelete(t *testing.T) {
	bs, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	content := "hello, st0r4g9"
	res, err := bs.Write(7, strings.NewReader(content))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if res.Size != int64(len(content)) {
		t.Fatalf("size: want %d, got %d", len(content), res.Size)
	}
	sum := sha256.Sum256([]byte(content))
	if res.ETag != hex.EncodeToString(sum[:]) {
		t.Fatalf("etag mismatch: %s", res.ETag)
	}

	f, err := bs.Open(res.RelPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	got, _ := io.ReadAll(f)
	f.Close()
	if string(got) != content {
		t.Fatalf("read back: want %q, got %q", content, got)
	}

	if err := bs.Delete(res.RelPath); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := bs.Open(res.RelPath); err == nil {
		t.Fatal("expected open to fail after delete")
	}
	// Deleting a missing blob is a no-op.
	if err := bs.Delete(res.RelPath); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
}

func TestWriteEmptyObject(t *testing.T) {
	bs, err := storage.New(t.TempDir())
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	res, err := bs.Write(1, strings.NewReader(""))
	if err != nil {
		t.Fatalf("write empty: %v", err)
	}
	if res.Size != 0 {
		t.Fatalf("empty size: want 0, got %d", res.Size)
	}
	f, _ := bs.Open(res.RelPath)
	got, _ := io.ReadAll(f)
	f.Close()
	if len(got) != 0 {
		t.Fatalf("want empty read, got %d bytes", len(got))
	}
}
