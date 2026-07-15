package api

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/zignd/st0r4g9/internal/reqctx"
)

// requestLogger is the outermost middleware: it emits one structured access-log
// line per request once the response is ready. The level is chosen by status —
// 5xx → error, 4xx → warn, else info — and any detail attached via reqctx.SetError
// (e.g. the reason for a 500) is included.
func requestLogger(log *slog.Logger) fiber.Handler {
	return func(c fiber.Ctx) error {
		start := time.Now()

		// A quiet "incoming" line, only visible at debug level.
		log.Debug("request in", "method", c.Method(), "path", c.Path())

		chainErr := c.Next()

		status := c.Response().StatusCode()
		// Buffered responses (JSON) expose their bytes via Body(); streamed
		// downloads (SendStream) leave Body() empty but set Content-Length.
		size := len(c.Response().Body())
		if size == 0 {
			if cl := c.Response().Header.ContentLength(); cl > 0 {
				size = cl
			}
		}

		attrs := []any{
			"method", c.Method(),
			"path", c.Path(),
			"status", status,
			"latency_ms", float64(time.Since(start).Microseconds()) / 1000.0,
			"bytes", size,
			"ip", c.IP(),
		}
		if q := string(c.Request().URI().QueryString()); q != "" {
			attrs = append(attrs, "query", q)
		}
		if detail := reqctx.Error(c); detail != "" {
			attrs = append(attrs, "error", detail)
		}

		switch {
		case status >= 500:
			log.Error("request", attrs...)
		case status >= 400:
			log.Warn("request", attrs...)
		default:
			log.Info("request", attrs...)
		}
		return chainErr
	}
}
