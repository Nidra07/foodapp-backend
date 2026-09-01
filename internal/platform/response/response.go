// Package response defines the single JSON envelope shape used by every
// API endpoint across every module, so client apps (Flutter x3 + Next.js)
// only need one response parser.
package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
	apperr "github.com/foodapp/backend/internal/platform/errors"
)

type Envelope struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorBody  `json:"error,omitempty"`
	Meta    *Meta       `json:"meta,omitempty"`
}

type ErrorBody struct {
	Code    string                 `json:"code"`
	Message string                 `json:"message"`
	Details map[string]interface{} `json:"details,omitempty"`
}

type Meta struct {
	Page       int   `json:"page,omitempty"`
	PageSize   int   `json:"page_size,omitempty"`
	TotalCount int64 `json:"total_count,omitempty"`
	TotalPages int   `json:"total_pages,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
}

func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Envelope{Success: true, Data: data})
}

func Created(c *gin.Context, data interface{}) {
	c.JSON(http.StatusCreated, Envelope{Success: true, Data: data})
}

func NoContent(c *gin.Context) {
	c.Status(http.StatusNoContent)
}

func Paginated(c *gin.Context, data interface{}, meta Meta) {
	c.JSON(http.StatusOK, Envelope{Success: true, Data: data, Meta: &meta})
}

// Error maps an error into the standard envelope. Unknown errors are
// coerced into a 500 without leaking internal details to the client.
func Error(c *gin.Context, err error) {
	ae, ok := apperr.As(err)
	if !ok {
		ae = apperr.Internal(err)
	}

	c.JSON(ae.HTTPStatus(), Envelope{
		Success: false,
		Error: &ErrorBody{
			Code:    string(ae.Code),
			Message: ae.Message,
			Details: ae.Details,
		},
	})
}
