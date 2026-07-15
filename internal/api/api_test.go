package api_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/zignd/st0r4g9/internal/api"
	"github.com/zignd/st0r4g9/internal/storage"
	"github.com/zignd/st0r4g9/internal/store"
)

// quietLogger discards log output so tests stay quiet.
func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// result is a captured HTTP response: status, body, and headers.
type result struct {
	Code   int
	body   string
	header http.Header
}

func (r result) String() string      { return r.body }
func (r result) Header() http.Header { return r.header }

type testEnv struct {
	t   *testing.T
	app *fiber.App
	key string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	blobs, err := storage.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("open blobs: %v", err)
	}
	env := &testEnv{t: t, app: api.New(st, blobs, quietLogger())}
	env.key = env.createKey()
	return env
}

// do issues a request through the Fiber app. headers are "Key: Value" strings.
func (e *testEnv) do(method, target, body string, headers ...string) result {
	e.t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, r)
	for _, h := range headers {
		parts := strings.SplitN(h, ": ", 2)
		req.Header.Set(parts[0], parts[1])
	}
	resp, err := e.app.Test(req, fiber.TestConfig{Timeout: 0})
	if err != nil {
		e.t.Fatalf("%s %s: %v", method, target, err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return result{Code: resp.StatusCode, body: string(b), header: resp.Header}
}

func (e *testEnv) auth() string { return "Bearer " + e.key }

func (e *testEnv) createKey() string {
	e.t.Helper()
	req := httptest.NewRequest(fiber.MethodPost, "/api-keys", strings.NewReader(`{"name":"test"}`))
	req.Header.Set(fiber.HeaderContentType, "application/json")
	resp, err := e.app.Test(req)
	if err != nil {
		e.t.Fatalf("create key: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusCreated {
		e.t.Fatalf("create key status: %d", resp.StatusCode)
	}
	var out struct {
		Key string `json:"key"`
	}
	json.NewDecoder(resp.Body).Decode(&out)
	if out.Key == "" {
		e.t.Fatal("empty key returned")
	}
	return out.Key
}

func TestHappyPath(t *testing.T) {
	e := newTestEnv(t)

	// Create bucket.
	if rec := e.do(fiber.MethodPut, "/photos", "", "Authorization: "+e.auth()); rec.Code != fiber.StatusCreated {
		t.Fatalf("create bucket: want 201, got %d (%s)", rec.Code, rec.String())
	}

	// Put object with metadata + annotation.
	rec := e.do(fiber.MethodPut, "/photos/a/b.txt", "hello world",
		"Authorization: "+e.auth(),
		"Content-Type: text/plain",
		"X-Str-Meta-Author: igor",
		"X-Str-Annotation-project: demo",
	)
	if rec.Code != fiber.StatusCreated {
		t.Fatalf("put object: want 201, got %d (%s)", rec.Code, rec.String())
	}

	// Get object back — bytes and headers.
	rec = e.do(fiber.MethodGet, "/photos/a/b.txt", "", "Authorization: "+e.auth())
	if rec.Code != fiber.StatusOK {
		t.Fatalf("get object: want 200, got %d", rec.Code)
	}
	if got := rec.String(); got != "hello world" {
		t.Fatalf("get body: want 'hello world', got %q", got)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain" {
		t.Fatalf("content-type: want text/plain, got %q", ct)
	}
	if rec.Header().Get("X-Str-Meta-Author") != "igor" {
		t.Fatalf("meta header missing: %v", rec.Header())
	}

	// HEAD object.
	rec = e.do(fiber.MethodHead, "/photos/a/b.txt", "", "Authorization: "+e.auth())
	if rec.Code != fiber.StatusOK {
		t.Fatalf("head object: want 200, got %d", rec.Code)
	}

	// List with prefix.
	rec = e.do(fiber.MethodGet, "/photos?prefix=a/", "", "Authorization: "+e.auth())
	if rec.Code != fiber.StatusOK || !strings.Contains(rec.String(), "a/b.txt") {
		t.Fatalf("list prefix: %d %s", rec.Code, rec.String())
	}

	// Query by annotation.
	rec = e.do(fiber.MethodGet, "/photos?annotation.project=demo", "", "Authorization: "+e.auth())
	if rec.Code != fiber.StatusOK || !strings.Contains(rec.String(), "a/b.txt") {
		t.Fatalf("annotation query: %d %s", rec.Code, rec.String())
	}
	// Non-matching annotation returns no objects.
	rec = e.do(fiber.MethodGet, "/photos?annotation.project=nope", "", "Authorization: "+e.auth())
	if strings.Contains(rec.String(), "a/b.txt") {
		t.Fatalf("annotation query should exclude: %s", rec.String())
	}

	// Delete object.
	if rec := e.do(fiber.MethodDelete, "/photos/a/b.txt", "", "Authorization: "+e.auth()); rec.Code != fiber.StatusNoContent {
		t.Fatalf("delete object: want 204, got %d", rec.Code)
	}
	if rec := e.do(fiber.MethodGet, "/photos/a/b.txt", "", "Authorization: "+e.auth()); rec.Code != fiber.StatusNotFound {
		t.Fatalf("get deleted: want 404, got %d", rec.Code)
	}
}

func TestErrorCases(t *testing.T) {
	e := newTestEnv(t)

	// No auth.
	if rec := e.do(fiber.MethodGet, "/", ""); rec.Code != fiber.StatusUnauthorized {
		t.Fatalf("no auth: want 401, got %d", rec.Code)
	}

	// Object in a bucket that doesn't exist.
	if rec := e.do(fiber.MethodGet, "/ghost/x", "", "Authorization: "+e.auth()); rec.Code != fiber.StatusNotFound {
		t.Fatalf("missing bucket: want 404, got %d", rec.Code)
	}

	// Duplicate bucket in the same namespace.
	e.do(fiber.MethodPut, "/photos", "", "Authorization: "+e.auth())
	if rec := e.do(fiber.MethodPut, "/photos", "", "Authorization: "+e.auth()); rec.Code != fiber.StatusConflict {
		t.Fatalf("dup bucket: want 409, got %d", rec.Code)
	}

	// Delete a non-empty bucket without force → 409; with force → 204.
	e.do(fiber.MethodPut, "/photos/x.txt", "data", "Authorization: "+e.auth())
	if rec := e.do(fiber.MethodDelete, "/photos", "", "Authorization: "+e.auth()); rec.Code != fiber.StatusConflict {
		t.Fatalf("delete non-empty: want 409, got %d", rec.Code)
	}
	if rec := e.do(fiber.MethodDelete, "/photos?force=true", "", "Authorization: "+e.auth()); rec.Code != fiber.StatusNoContent {
		t.Fatalf("force delete: want 204, got %d", rec.Code)
	}

	// Invalid bucket name.
	if rec := e.do(fiber.MethodPut, "/AB", "", "Authorization: "+e.auth()); rec.Code != fiber.StatusBadRequest {
		t.Fatalf("bad bucket name: want 400, got %d", rec.Code)
	}
}

func TestNamespaceIsolationOverHTTP(t *testing.T) {
	e := newTestEnv(t)
	other := &testEnv{t: t, app: e.app}
	other.key = other.createKey()

	// Both keys create "photos"; both should succeed (isolated).
	if rec := e.do(fiber.MethodPut, "/photos", "", "Authorization: "+e.auth()); rec.Code != fiber.StatusCreated {
		t.Fatalf("key A create: %d", rec.Code)
	}
	if rec := other.do(fiber.MethodPut, "/photos", "", "Authorization: "+other.auth()); rec.Code != fiber.StatusCreated {
		t.Fatalf("key B create: %d", rec.Code)
	}

	// A writes an object; B must not see it in its own "photos".
	e.do(fiber.MethodPut, "/photos/secret.txt", "A only", "Authorization: "+e.auth())
	rec := other.do(fiber.MethodGet, "/photos/secret.txt", "", "Authorization: "+other.auth())
	if rec.Code != fiber.StatusNotFound {
		t.Fatalf("cross-namespace read: want 404, got %d", rec.Code)
	}
}
