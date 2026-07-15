package auth

import (
	"errors"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/zignd/st0r4g9/internal/reqctx"
	"github.com/zignd/st0r4g9/internal/store"
)

// localsKey is the (unexported, collision-free) key under which the
// authenticated API key is stored on the Fiber context.
type localsKey struct{}

// Resolver looks up an API key by the SHA-256 hash of its raw secret.
// *store.Store satisfies this; it is an interface for testability.
type Resolver interface {
	APIKeyByHash(keyHash string) (store.APIKey, error)
}

// Middleware authenticates each request via the Authorization header
// ("Bearer <key>") and stashes the resolved API key on the context. Requests
// without a valid key get a 401 JSON error and are not passed downstream.
func Middleware(r Resolver) fiber.Handler {
	return func(c fiber.Ctx) error {
		raw := bearerToken(c.Get(fiber.HeaderAuthorization))
		if raw == "" {
			return unauthorized(c, "missing or malformed Authorization header (expected 'Bearer <key>')")
		}
		key, err := r.APIKeyByHash(Hash(raw))
		if errors.Is(err, store.ErrNotFound) {
			return unauthorized(c, "invalid API key")
		}
		if err != nil {
			reqctx.SetError(c, err.Error())
			return c.Status(fiber.StatusInternalServerError).
				JSON(fiber.Map{"error": fiber.Map{"code": "InternalError", "message": err.Error()}})
		}
		c.Locals(localsKey{}, key)
		return c.Next()
	}
}

// KeyFromContext returns the authenticated API key placed on the context by
// Middleware. The bool is false when the request was not authenticated.
func KeyFromContext(c fiber.Ctx) (store.APIKey, bool) {
	v, ok := c.Locals(localsKey{}).(store.APIKey)
	return v, ok
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return ""
	}
	return strings.TrimSpace(header[len(prefix):])
}

func unauthorized(c fiber.Ctx, msg string) error {
	return c.Status(fiber.StatusUnauthorized).
		JSON(fiber.Map{"error": fiber.Map{"code": "Unauthorized", "message": msg}})
}
