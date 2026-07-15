package api

import (
	"github.com/gofiber/fiber/v3"

	"github.com/zignd/st0r4g9/internal/auth"
)

type createAPIKeyRequest struct {
	Name string `json:"name"`
}

type createAPIKeyResponse struct {
	// Key is the raw secret. It is shown exactly once and never stored in the
	// clear — save it now.
	Key       string `json:"key"`
	KeyPrefix string `json:"keyPrefix"`
	Name      string `json:"name"`
	CreatedAt string `json:"createdAt"`
}

// createAPIKey mints a new API key. This is the only unauthenticated endpoint.
func (h *handlers) createAPIKey(c fiber.Ctx) error {
	var req createAPIKeyRequest
	// Body is optional; ignore parse errors on an empty/absent body.
	if len(c.Body()) > 0 {
		if err := c.Bind().Body(&req); err != nil {
			return errInvalidRequest("invalid JSON body").write(c)
		}
	}

	raw, err := auth.GenerateKey()
	if err != nil {
		return respond(c, errInternal(err))
	}

	key, err := h.store.CreateAPIKey(req.Name, auth.Hash(raw), auth.DisplayPrefix(raw))
	if err != nil {
		return respond(c, errInternal(err))
	}

	return c.Status(fiber.StatusCreated).JSON(createAPIKeyResponse{
		Key:       raw,
		KeyPrefix: key.KeyPrefix,
		Name:      key.Name,
		CreatedAt: key.CreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"),
	})
}
