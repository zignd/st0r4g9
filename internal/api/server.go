// Package api wires the HTTP surface (Fiber) onto the store and blob storage.
package api

import (
	"log/slog"

	"github.com/gofiber/fiber/v3"

	"github.com/zignd/st0r4g9/internal/auth"
	"github.com/zignd/st0r4g9/internal/reqctx"
	"github.com/zignd/st0r4g9/internal/storage"
	"github.com/zignd/st0r4g9/internal/store"
)

// handlers bundles the dependencies shared by every route handler.
type handlers struct {
	store *store.Store
	blobs *storage.BlobStore
}

// New builds the Fiber app with all routes registered. POST /api-keys is open;
// every other route sits behind the auth middleware. log receives one
// structured access-log line per request.
func New(st *store.Store, blobs *storage.BlobStore, log *slog.Logger) *fiber.App {
	app := fiber.New(fiber.Config{
		// Stream request bodies straight to disk instead of buffering in RAM,
		// so large object uploads stay memory-flat.
		StreamRequestBody: true,
		// 5 TiB, matching S3's documented per-object ceiling.
		BodyLimit: 5 << 40,
	})

	h := &handlers{store: st, blobs: blobs}

	// Access logging wraps every route, including the open one below.
	app.Use(requestLogger(log))

	// Open endpoint: mint an API key.
	app.Post("/api-keys", h.createAPIKey)

	// Everything below requires a valid key.
	app.Use(auth.Middleware(st))

	app.Get("/", h.listBuckets)

	app.Put("/:bucket", h.createBucket)
	app.Head("/:bucket", h.headBucket)
	app.Delete("/:bucket", h.deleteBucket)
	app.Get("/:bucket", h.listObjects)

	app.Put("/:bucket/*", h.putObject)
	app.Get("/:bucket/*", h.getObject)
	app.Head("/:bucket/*", h.headObject)
	app.Delete("/:bucket/*", h.deleteObject)

	return app
}

// respond writes an apiError if err is one, otherwise a generic 500. Server-side
// failures (5xx) attach their detail to the request so the access log records
// what went wrong.
func respond(c fiber.Ctx, err error) error {
	if ae, ok := err.(apiError); ok {
		if ae.Status >= 500 {
			reqctx.SetError(c, ae.Message)
		}
		return ae.write(c)
	}
	reqctx.SetError(c, err.Error())
	return errInternal(err).write(c)
}
