// Package reqctx carries per-request annotations on the Fiber context that the
// access-log middleware reads. Keeping it in its own package lets both the api
// handlers and the auth middleware attach an error detail without importing the
// logging middleware (which would create an import cycle).
package reqctx

import "github.com/gofiber/fiber/v3"

type errKey struct{}

// SetError records a human-readable failure detail for the current request so
// the access log can include it. Safe to call more than once (last wins).
func SetError(c fiber.Ctx, msg string) {
	c.Locals(errKey{}, msg)
}

// Error returns the detail set by SetError, or "" if none was set.
func Error(c fiber.Ctx) string {
	if s, ok := c.Locals(errKey{}).(string); ok {
		return s
	}
	return ""
}
