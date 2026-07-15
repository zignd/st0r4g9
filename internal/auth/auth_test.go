package auth_test

import (
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"

	"github.com/zignd/st0r4g9/internal/auth"
	"github.com/zignd/st0r4g9/internal/store"
)

func TestGenerateKeyFormatAndUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		k, err := auth.GenerateKey()
		if err != nil {
			t.Fatalf("generate: %v", err)
		}
		if !strings.HasPrefix(k, auth.KeyPrefix) {
			t.Fatalf("key missing prefix: %q", k)
		}
		if seen[k] {
			t.Fatalf("duplicate key generated: %q", k)
		}
		seen[k] = true
	}
}

func TestHashDeterministicAndDistinct(t *testing.T) {
	first := auth.Hash("abc")
	again := auth.Hash("abc")
	if first != again {
		t.Fatal("hash not deterministic")
	}
	if first == auth.Hash("abd") {
		t.Fatal("distinct inputs hashed the same")
	}
}

// fakeResolver resolves exactly one known hash to a fixed key.
type fakeResolver struct{ validHash string }

func (f fakeResolver) APIKeyByHash(hash string) (store.APIKey, error) {
	if hash == f.validHash {
		return store.APIKey{ID: 42, Name: "test"}, nil
	}
	return store.APIKey{}, store.ErrNotFound
}

func TestMiddleware(t *testing.T) {
	const raw = "st0r_secret"
	app := fiber.New()
	app.Use(auth.Middleware(fakeResolver{validHash: auth.Hash(raw)}))
	app.Get("/", func(c fiber.Ctx) error {
		key, ok := auth.KeyFromContext(c)
		if !ok {
			return c.SendStatus(fiber.StatusInternalServerError)
		}
		return c.SendString(key.Name)
	})

	cases := []struct {
		name       string
		header     string
		wantStatus int
	}{
		{"valid", "Bearer " + raw, fiber.StatusOK},
		{"missing", "", fiber.StatusUnauthorized},
		{"malformed", "Token " + raw, fiber.StatusUnauthorized},
		{"wrong key", "Bearer st0r_wrong", fiber.StatusUnauthorized},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(fiber.MethodGet, "/", nil)
			if tc.header != "" {
				req.Header.Set(fiber.HeaderAuthorization, tc.header)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("test request: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.wantStatus {
				t.Fatalf("status: want %d, got %d", tc.wantStatus, resp.StatusCode)
			}
			if tc.wantStatus == fiber.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				if string(body) != "test" {
					t.Fatalf("body: want key name 'test', got %q", body)
				}
			}
		})
	}
}
