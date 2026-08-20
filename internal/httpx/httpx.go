// Package httpx provides shared HTTP helpers: the unified API error envelope,
// JSON encoding/decoding and request utilities used by every module.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// ErrorBody is the single error envelope returned by every API endpoint.
type ErrorBody struct {
	Error ErrorDetail `json:"error"`
}

// ErrorDetail carries a machine-readable code, a human message and the
// request id for support correlation. Internal details never leak here.
type ErrorDetail struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

// JSON writes v as a JSON response with the given status code.
func JSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// Error writes the unified error envelope.
func Error(w http.ResponseWriter, r *http.Request, status int, code, message string) {
	JSON(w, status, ErrorBody{Error: ErrorDetail{
		Code:      code,
		Message:   message,
		RequestID: middleware.GetReqID(r.Context()),
	}})
}

// Internal writes a generic 500 without leaking the underlying error.
func Internal(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusInternalServerError, "INTERNAL", "Internal server error")
}

const maxBodyBytes = 1 << 20 // 1 MiB default request body cap

// Decode reads a JSON request body into dst with a size cap and strict
// unknown-field handling. Returns a client-friendly error message.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return errors.New("request body too large")
		}
		return errors.New("invalid JSON body")
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return errors.New("request body must contain a single JSON object")
	}
	return nil
}

// ClientIP returns the request's client IP as set by chi RealIP middleware.
func ClientIP(r *http.Request) string {
	return r.RemoteAddr
}
