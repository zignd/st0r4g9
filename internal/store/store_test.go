package store_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/zignd/st0r4g9/internal/store"
)

func newStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func mustKey(t *testing.T, st *store.Store, name string) store.APIKey {
	t.Helper()
	k, err := st.CreateAPIKey(name, "hash-"+name, "st0r_"+name)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	return k
}

func TestBucketPerKeyIsolation(t *testing.T) {
	st := newStore(t)
	keyA := mustKey(t, st, "a")
	keyB := mustKey(t, st, "b")

	if _, err := st.CreateBucket(keyA.ID, "photos"); err != nil {
		t.Fatalf("A create photos: %v", err)
	}
	// Same name under a different key must succeed (isolated namespaces).
	if _, err := st.CreateBucket(keyB.ID, "photos"); err != nil {
		t.Fatalf("B create photos: %v", err)
	}
	// Same name under the same key must conflict.
	if _, err := st.CreateBucket(keyA.ID, "photos"); !errors.Is(err, store.ErrConflict) {
		t.Fatalf("A re-create photos: want ErrConflict, got %v", err)
	}

	// A cannot see B's bucket by way of its own namespace, and vice versa —
	// both lookups resolve to each caller's own bucket only.
	if _, err := st.BucketByName(keyA.ID, "photos"); err != nil {
		t.Fatalf("A lookup own: %v", err)
	}
	// B has its own "photos"; deleting A's must not affect B's.
	bA, _ := st.BucketByName(keyA.ID, "photos")
	bB, _ := st.BucketByName(keyB.ID, "photos")
	if bA.ID == bB.ID {
		t.Fatal("expected distinct bucket rows per key")
	}
}

func TestBucketListAndDelete(t *testing.T) {
	st := newStore(t)
	key := mustKey(t, st, "k")
	for _, n := range []string{"gamma", "alpha", "beta"} {
		if _, err := st.CreateBucket(key.ID, n); err != nil {
			t.Fatalf("create %s: %v", n, err)
		}
	}
	buckets, err := st.ListBuckets(key.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(buckets) != 3 || buckets[0].Name != "alpha" || buckets[2].Name != "gamma" {
		t.Fatalf("want sorted [alpha beta gamma], got %+v", buckets)
	}

	b, _ := st.BucketByName(key.ID, "beta")
	if err := st.DeleteBucket(b.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := st.BucketByName(key.ID, "beta"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("after delete: want ErrNotFound, got %v", err)
	}
}

func TestObjectPutGetOverwriteDelete(t *testing.T) {
	st := newStore(t)
	key := mustKey(t, st, "k")
	bucket, _ := st.CreateBucket(key.ID, "photos")

	obj, oldPath, err := st.PutObject(store.PutObjectInput{
		BucketID:    bucket.ID,
		Key:         "a/b.txt",
		Size:        3,
		ContentType: "text/plain",
		ETag:        "etag1",
		StoragePath: "1/blob1",
		Metadata:    map[string]string{"author": "igor"},
		Annotations: map[string]string{"project": "demo"},
	})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if oldPath != "" {
		t.Fatalf("first put should report no old path, got %q", oldPath)
	}

	got, err := st.GetObject(bucket.ID, "a/b.txt")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ETag != "etag1" || got.Metadata["author"] != "igor" || got.Annotations["project"] != "demo" {
		t.Fatalf("unexpected object: %+v", got)
	}

	// Overwrite: old path reported for cleanup, created_at preserved.
	obj2, oldPath2, err := st.PutObject(store.PutObjectInput{
		BucketID:    bucket.ID,
		Key:         "a/b.txt",
		Size:        5,
		ContentType: "text/plain",
		ETag:        "etag2",
		StoragePath: "1/blob2",
	})
	if err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	if oldPath2 != "1/blob1" {
		t.Fatalf("overwrite should report old path 1/blob1, got %q", oldPath2)
	}
	if !obj2.CreatedAt.Equal(obj.CreatedAt) {
		t.Fatalf("overwrite should preserve created_at: %v vs %v", obj2.CreatedAt, obj.CreatedAt)
	}
	got2, _ := st.GetObject(bucket.ID, "a/b.txt")
	if got2.ETag != "etag2" || len(got2.Metadata) != 0 {
		t.Fatalf("overwrite should replace metadata: %+v", got2)
	}

	// Delete returns the current blob path.
	path, err := st.DeleteObject(bucket.ID, "a/b.txt")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if path != "1/blob2" {
		t.Fatalf("delete path: want 1/blob2, got %q", path)
	}
	if _, err := st.GetObject(bucket.ID, "a/b.txt"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("after delete: want ErrNotFound, got %v", err)
	}
}

func TestListObjectsPrefixDelimiterAnnotations(t *testing.T) {
	st := newStore(t)
	key := mustKey(t, st, "k")
	bucket, _ := st.CreateBucket(key.ID, "photos")

	put := func(k string, ann map[string]string) {
		if _, _, err := st.PutObject(store.PutObjectInput{
			BucketID: bucket.ID, Key: k, Size: 1, ContentType: "text/plain",
			ETag: "e", StoragePath: "1/" + k, Annotations: ann,
		}); err != nil {
			t.Fatalf("put %s: %v", k, err)
		}
	}
	put("a/b/1.txt", map[string]string{"project": "demo"})
	put("a/b/2.txt", nil)
	put("a/c.txt", map[string]string{"project": "other"})
	put("root.txt", map[string]string{"project": "demo"})

	// Prefix filter.
	res, err := st.ListObjects(store.ListObjectsInput{BucketID: bucket.ID, Prefix: "a/"})
	if err != nil {
		t.Fatalf("list prefix: %v", err)
	}
	if len(res.Objects) != 3 {
		t.Fatalf("prefix a/: want 3 objects, got %d", len(res.Objects))
	}

	// Delimiter folding under prefix "a/": "a/b/" is a common prefix, "a/c.txt" a leaf.
	res, err = st.ListObjects(store.ListObjectsInput{BucketID: bucket.ID, Prefix: "a/", Delimiter: "/"})
	if err != nil {
		t.Fatalf("list delimiter: %v", err)
	}
	if len(res.CommonPrefixes) != 1 || res.CommonPrefixes[0] != "a/b/" {
		t.Fatalf("common prefixes: want [a/b/], got %v", res.CommonPrefixes)
	}
	if len(res.Objects) != 1 || res.Objects[0].Key != "a/c.txt" {
		t.Fatalf("delimiter leaves: want [a/c.txt], got %+v", res.Objects)
	}

	// Annotation filter.
	res, err = st.ListObjects(store.ListObjectsInput{
		BucketID: bucket.ID, AnnotationFilters: map[string]string{"project": "demo"},
	})
	if err != nil {
		t.Fatalf("list annotation: %v", err)
	}
	if len(res.Objects) != 2 {
		t.Fatalf("annotation project=demo: want 2, got %d (%+v)", len(res.Objects), res.Objects)
	}
}
