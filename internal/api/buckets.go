package api

import (
	"errors"

	"github.com/gofiber/fiber/v3"

	"github.com/zignd/st0r4g9/internal/auth"
	"github.com/zignd/st0r4g9/internal/store"
)

// createBucket handles PUT /:bucket.
func (h *handlers) createBucket(c fiber.Ctx) error {
	key, _ := auth.KeyFromContext(c)
	name := c.Params("bucket")
	if err := validateBucketName(name); err != nil {
		return respond(c, err)
	}

	bucket, err := h.store.CreateBucket(key.ID, name)
	if errors.Is(err, store.ErrConflict) {
		return respond(c, errBucketAlreadyExists(name))
	}
	if err != nil {
		return respond(c, errInternal(err))
	}
	return c.Status(fiber.StatusCreated).JSON(bucket)
}

// listBuckets handles GET / — the caller's own bucket namespace.
func (h *handlers) listBuckets(c fiber.Ctx) error {
	key, _ := auth.KeyFromContext(c)
	buckets, err := h.store.ListBuckets(key.ID)
	if err != nil {
		return respond(c, errInternal(err))
	}
	return c.JSON(fiber.Map{"buckets": buckets})
}

// headBucket handles HEAD /:bucket.
func (h *handlers) headBucket(c fiber.Ctx) error {
	key, _ := auth.KeyFromContext(c)
	name := c.Params("bucket")
	_, err := h.store.BucketByName(key.ID, name)
	if errors.Is(err, store.ErrNotFound) {
		return c.SendStatus(fiber.StatusNotFound)
	}
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	return c.SendStatus(fiber.StatusOK)
}

// deleteBucket handles DELETE /:bucket. Refuses a non-empty bucket unless
// ?force=true, in which case every contained object (row + blob) is removed.
func (h *handlers) deleteBucket(c fiber.Ctx) error {
	key, _ := auth.KeyFromContext(c)
	name := c.Params("bucket")

	bucket, err := h.store.BucketByName(key.ID, name)
	if errors.Is(err, store.ErrNotFound) {
		return respond(c, errNoSuchBucket(name))
	}
	if err != nil {
		return respond(c, errInternal(err))
	}

	count, err := h.store.CountObjects(bucket.ID)
	if err != nil {
		return respond(c, errInternal(err))
	}
	force := c.Query("force") == "true"
	if count > 0 && !force {
		return respond(c, errBucketNotEmpty(name))
	}

	// Gather blob paths before the DB cascade drops the rows.
	paths, err := h.store.StoragePaths(bucket.ID)
	if err != nil {
		return respond(c, errInternal(err))
	}
	if err := h.store.DeleteBucket(bucket.ID); err != nil {
		return respond(c, errInternal(err))
	}
	for _, p := range paths {
		_ = h.blobs.Delete(p)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// validateBucketName enforces simple, S3-ish naming rules.
func validateBucketName(name string) error {
	if len(name) < 3 || len(name) > 63 {
		return errInvalidRequest("bucket name must be 3-63 characters")
	}
	for _, r := range name {
		isLower := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		if !isLower && !isDigit && r != '-' && r != '.' {
			return errInvalidRequest("bucket name may contain only lowercase letters, digits, '-' and '.'")
		}
	}
	return nil
}
