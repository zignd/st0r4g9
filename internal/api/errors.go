package api

import "github.com/gofiber/fiber/v3"

// apiError is a structured API error rendered as
// {"error":{"code":"...","message":"..."}} with an HTTP status.
type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e apiError) Error() string { return e.Code + ": " + e.Message }

// write renders the error onto the Fiber context as a JSON body.
func (e apiError) write(c fiber.Ctx) error {
	return c.Status(e.Status).JSON(fiber.Map{
		"error": fiber.Map{"code": e.Code, "message": e.Message},
	})
}

// Constructors for the S3-flavored error conditions this service raises.

func errNoSuchBucket(name string) apiError {
	return apiError{fiber.StatusNotFound, "NoSuchBucket", "no such bucket: " + name}
}

func errNoSuchKey(key string) apiError {
	return apiError{fiber.StatusNotFound, "NoSuchKey", "no such object: " + key}
}

func errBucketAlreadyExists(name string) apiError {
	return apiError{fiber.StatusConflict, "BucketAlreadyExists", "bucket already exists in your namespace: " + name}
}

func errBucketNotEmpty(name string) apiError {
	return apiError{fiber.StatusConflict, "BucketNotEmpty", "bucket is not empty (use ?force=true to delete anyway): " + name}
}

func errInvalidRequest(msg string) apiError {
	return apiError{fiber.StatusBadRequest, "InvalidRequest", msg}
}

func errInternal(err error) apiError {
	return apiError{fiber.StatusInternalServerError, "InternalError", err.Error()}
}
