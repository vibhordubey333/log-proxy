package proxy

import (
	"errors"
	"github.com/gin-gonic/gin"
	"io"
	"net/http"
	"os"
	"strconv"
)

// Handler wires the Cache into Gin routes.
type Handler struct {
	Cache *Cache
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	// GET returns the requested log content in the response body.
	r.GET("/logs/:build_id", h.HandleGet)
	// HEAD returns only headers for the requested log, without the body.
	// This lets clients check availability or metadata without downloading it.
	r.HEAD("/logs/:build_id", h.HandleHead)
}

func (h *Handler) HandleHead(c *gin.Context) {
	buildID := c.Param("build_id")

	cached, err := h.Cache.Resolve(c.Request.Context(), buildID)
	if err != nil {
		writeError(c, err)
		return
	}

	info, err := os.Stat(cached.Path)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Header("Content-Type", cached.ContentType)
	c.Header("Content-Length", strconv.FormatInt(info.Size(), 10))
	c.Status(http.StatusOK)
}

func (h *Handler) HandleGet(c *gin.Context) {
	buildID := c.Param("build_id")

	offset, limit, err := parseRange(c)
	if err != nil {
		c.String(http.StatusBadRequest, err.Error())
		return
	}

	cached, err := h.Cache.Resolve(c.Request.Context(), buildID)
	if err != nil {
		writeError(c, err)
		return
	}

	file, err := os.Open(cached.Path)
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}
	size := info.Size()

	if offset > size {
		c.String(http.StatusRequestedRangeNotSatisfiable, "offset %d beyond file size %d", offset, size)
		return
	}

	remaining := size - offset
	if limit <= 0 || limit > remaining {
		limit = remaining
	}

	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	c.Header("Content-Type", cached.ContentType)
	c.Header("Content-Length", strconv.FormatInt(limit, 10))
	c.Status(http.StatusOK)
	io.CopyN(c.Writer, file, limit)
}

// parseRange reads optional offset/limit query params, defaulting
// offset to 0 and limit to "rest of file" (signalled here as 0,
// resolved against actual file size by the caller).
func parseRange(c *gin.Context) (offset, limit int64, err error) {
	// offset is the number of records to skip before returning results.
	// It is optional; when omitted, the default value remains 0.
	if v := c.Query("offset"); v != "" {
		offset, err = strconv.ParseInt(v, 10, 64)
		if err != nil || offset < 0 {
			return 0, 0, errInvalidParam("offset")
		}
	}
	// limit is the maximum number of records to return.
	// It is optional; when omitted, the default value remains 0,
	if v := c.Query("limit"); v != "" {
		limit, err = strconv.ParseInt(v, 10, 64)
		if err != nil || limit < 0 {
			return 0, 0, errInvalidParam("limit")
		}
	}
	return offset, limit, nil
}

func errInvalidParam(name string) error {
	return errors.New("invalid " + name + ": must be a non-negative integer")
}

func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInvalidBuildID):
		c.String(http.StatusBadRequest, err.Error())
	case errors.Is(err, ErrBuildNotFound):
		c.String(http.StatusNotFound, err.Error())
	case errors.Is(err, ErrUpstreamUnavailable):
		c.String(http.StatusBadGateway, err.Error())
	default:
		c.String(http.StatusInternalServerError, err.Error())
	}
}
