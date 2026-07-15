package api

import (
	"bytes"
	"errors"
	"io"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/zignd/st0r4g9/internal/auth"
	"github.com/zignd/st0r4g9/internal/store"
)

const (
	metaHeaderPrefix       = "x-str-meta-"
	annotationHeaderPrefix = "x-str-annotation-"
	annotationQueryPrefix  = "annotation."
	defaultContentType     = "application/octet-stream"
)

// putObject handles PUT /:bucket/* — create or overwrite an object. The body is
// streamed to disk; X-Str-Meta-* and X-Str-Annotation-* headers become the
// object's metadata and annotations.
func (h *handlers) putObject(c fiber.Ctx) error {
	bucket, err := h.resolveBucket(c)
	if err != nil {
		return respond(c, err)
	}
	key := c.Params("*")
	if key == "" {
		return respond(c, errInvalidRequest("object key must not be empty"))
	}

	written, err := h.blobs.Write(bucket.ID, requestBodyReader(c))
	if err != nil {
		return respond(c, errInternal(err))
	}

	contentType := c.Get(fiber.HeaderContentType)
	if contentType == "" {
		contentType = defaultContentType
	}
	meta, annotations := headerPairs(c)

	obj, oldPath, err := h.store.PutObject(store.PutObjectInput{
		BucketID:    bucket.ID,
		Key:         key,
		Size:        written.Size,
		ContentType: contentType,
		ETag:        written.ETag,
		StoragePath: written.RelPath,
		Metadata:    meta,
		Annotations: annotations,
	})
	if err != nil {
		// Roll back the just-written blob so we don't orphan it.
		_ = h.blobs.Delete(written.RelPath)
		return respond(c, errInternal(err))
	}
	// An overwrite leaves the previous blob behind; remove it.
	if oldPath != "" && oldPath != written.RelPath {
		_ = h.blobs.Delete(oldPath)
	}

	c.Set(fiber.HeaderETag, quoteETag(obj.ETag))
	return c.Status(fiber.StatusCreated).JSON(obj)
}

// getObject handles GET /:bucket/* — stream the object bytes back.
func (h *handlers) getObject(c fiber.Ctx) error {
	bucket, err := h.resolveBucket(c)
	if err != nil {
		return respond(c, err)
	}
	key := c.Params("*")

	obj, err := h.store.GetObject(bucket.ID, key)
	if errors.Is(err, store.ErrNotFound) {
		return respond(c, errNoSuchKey(key))
	}
	if err != nil {
		return respond(c, errInternal(err))
	}

	f, err := h.blobs.Open(obj.StoragePath)
	if err != nil {
		return respond(c, errInternal(err))
	}
	setObjectHeaders(c, obj)
	// SendStream closes f (an io.Closer) once the body has been written.
	return c.SendStream(f, int(obj.Size))
}

// headObject handles HEAD /:bucket/* — metadata headers only, no body.
func (h *handlers) headObject(c fiber.Ctx) error {
	bucket, err := h.resolveBucket(c)
	if err != nil {
		if _, ok := err.(apiError); ok {
			return c.SendStatus(fiber.StatusNotFound)
		}
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	key := c.Params("*")

	obj, err := h.store.GetObject(bucket.ID, key)
	if errors.Is(err, store.ErrNotFound) {
		return c.SendStatus(fiber.StatusNotFound)
	}
	if err != nil {
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	setObjectHeaders(c, obj)
	// Report the object's real size without writing a body. Setting the length
	// on the raw response header (rather than SendStatus, which would emit the
	// status text as the body) keeps Content-Length accurate for HEAD.
	c.Response().Header.SetContentLength(int(obj.Size))
	c.Status(fiber.StatusOK)
	return nil
}

// deleteObject handles DELETE /:bucket/*.
func (h *handlers) deleteObject(c fiber.Ctx) error {
	bucket, err := h.resolveBucket(c)
	if err != nil {
		return respond(c, err)
	}
	key := c.Params("*")

	path, err := h.store.DeleteObject(bucket.ID, key)
	if errors.Is(err, store.ErrNotFound) {
		return respond(c, errNoSuchKey(key))
	}
	if err != nil {
		return respond(c, errInternal(err))
	}
	_ = h.blobs.Delete(path)
	return c.SendStatus(fiber.StatusNoContent)
}

// listObjects handles GET /:bucket with ?prefix, ?delimiter, and
// ?annotation.<name>=<value> filters.
func (h *handlers) listObjects(c fiber.Ctx) error {
	bucket, err := h.resolveBucket(c)
	if err != nil {
		return respond(c, err)
	}

	in := store.ListObjectsInput{
		BucketID:          bucket.ID,
		Prefix:            c.Query("prefix"),
		Delimiter:         c.Query("delimiter"),
		AnnotationFilters: map[string]string{},
	}
	for name, value := range c.Queries() {
		if strings.HasPrefix(name, annotationQueryPrefix) {
			in.AnnotationFilters[strings.TrimPrefix(name, annotationQueryPrefix)] = value
		}
	}

	res, err := h.store.ListObjects(in)
	if err != nil {
		return respond(c, errInternal(err))
	}
	return c.JSON(fiber.Map{
		"bucket":         bucket.Name,
		"prefix":         in.Prefix,
		"delimiter":      in.Delimiter,
		"objects":        res.Objects,
		"commonPrefixes": res.CommonPrefixes,
	})
}

// resolveBucket loads the bucket named in the path within the caller's
// namespace, returning errNoSuchBucket if the caller doesn't own it.
func (h *handlers) resolveBucket(c fiber.Ctx) (store.Bucket, error) {
	key, _ := auth.KeyFromContext(c)
	name := c.Params("bucket")
	bucket, err := h.store.BucketByName(key.ID, name)
	if errors.Is(err, store.ErrNotFound) {
		return store.Bucket{}, errNoSuchBucket(name)
	}
	if err != nil {
		return store.Bucket{}, errInternal(err)
	}
	return bucket, nil
}

// setObjectHeaders sets the standard + custom-metadata response headers.
func setObjectHeaders(c fiber.Ctx, obj store.Object) {
	c.Set(fiber.HeaderContentType, obj.ContentType)
	c.Set(fiber.HeaderETag, quoteETag(obj.ETag))
	c.Set(fiber.HeaderLastModified, obj.UpdatedAt.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT"))
	for name, value := range obj.Metadata {
		c.Set("X-Str-Meta-"+name, value)
	}
	for name, value := range obj.Annotations {
		c.Set("X-Str-Annotation-"+name, value)
	}
}

// headerPairs extracts X-Str-Meta-* and X-Str-Annotation-* request headers into
// two maps keyed by the suffix after the prefix (lowercased).
func headerPairs(c fiber.Ctx) (meta, annotations map[string]string) {
	meta = map[string]string{}
	annotations = map[string]string{}
	for k, v := range c.Request().Header.All() {
		name := strings.ToLower(string(k))
		switch {
		case strings.HasPrefix(name, metaHeaderPrefix):
			meta[strings.TrimPrefix(name, metaHeaderPrefix)] = string(v)
		case strings.HasPrefix(name, annotationHeaderPrefix):
			annotations[strings.TrimPrefix(name, annotationHeaderPrefix)] = string(v)
		}
	}
	if len(meta) == 0 {
		meta = nil
	}
	if len(annotations) == 0 {
		annotations = nil
	}
	return meta, annotations
}

// requestBodyReader returns a streaming reader over the request body when
// available (large uploads), falling back to the buffered body.
func requestBodyReader(c fiber.Ctx) io.Reader {
	if bs := c.Request().BodyStream(); bs != nil {
		return bs
	}
	return bytes.NewReader(c.Body())
}

func quoteETag(etag string) string { return `"` + etag + `"` }
