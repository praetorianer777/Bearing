// Package httpapi is the HTTP surface: routing, middleware and handlers.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
)

// APIError is the single error shape every endpoint returns, so that clients have exactly one thing to parse.
type APIError struct {
	// Status is the HTTP status code; it is not serialised.
	Status int `json:"-"`
	// Code is a stable machine readable identifier such as "not_found".
	Code string `json:"code"`
	// Message is a sentence a person can read and act on.
	Message string `json:"message"`
	// Fields carries per-field validation messages keyed by field name.
	Fields map[string]string `json:"fields,omitempty"`
	// RequestID lets a user quote something we can find in the logs.
	RequestID string `json:"requestId,omitempty"`

	// cause is logged but never sent to the client.
	cause error
}

func (e *APIError) Error() string {
	if e.cause != nil {
		return e.Code + ": " + e.Message + ": " + e.cause.Error()
	}
	return e.Code + ": " + e.Message
}

func (e *APIError) Unwrap() error { return e.cause }

// errorEnvelope is how every failure is written.
type errorEnvelope struct {
	Error APIError `json:"error"`
}

// ErrNotFound is also what a caller gets for a resource behind a wall, so a wall does not say what is behind it.
func ErrNotFound() *APIError {
	return &APIError{Status: http.StatusNotFound, Code: "not_found", Message: "There is nothing here, or you may not see it."}
}

// ErrBadRequest is a request that cannot be read.
func ErrBadRequest(message string) *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: "bad_request", Message: message}
}

// ErrValidation names the fields that need attention.
func ErrValidation(fields map[string]string) *APIError {
	return &APIError{Status: http.StatusUnprocessableEntity, Code: "validation_failed", Message: "Some fields need attention.", Fields: fields}
}

// ErrUnauthorized is a request that needs somebody signed in.
func ErrUnauthorized() *APIError {
	return &APIError{Status: http.StatusUnauthorized, Code: "unauthorized", Message: "Sign in to continue."}
}

// ErrForbidden is somebody signed in who may not do this.
func ErrForbidden(message string) *APIError {
	return &APIError{Status: http.StatusForbidden, Code: "forbidden", Message: message}
}

// ErrCrossSite is a write a browser sent from another site.
func ErrCrossSite() *APIError {
	return &APIError{Status: http.StatusForbidden, Code: "cross_site", Message: "This request came from another site, so it was refused."}
}

// ErrConflict is a request the current state does not allow.
func ErrConflict(code, message string) *APIError {
	return &APIError{Status: http.StatusConflict, Code: code, Message: message}
}

// ErrMethodNotAllowed answers a path that exists with a method it does not take.
func ErrMethodNotAllowed() *APIError {
	return &APIError{Status: http.StatusMethodNotAllowed, Code: "method_not_allowed", Message: "This address does not take that method."}
}

// ErrUnavailable says the process cannot serve right now.
func ErrUnavailable(message string, cause error) *APIError {
	return &APIError{Status: http.StatusServiceUnavailable, Code: "unavailable", Message: message, cause: cause}
}

// ErrInternal hides what went wrong from the caller and keeps it for the log.
func ErrInternal(cause error) *APIError {
	return &APIError{Status: http.StatusInternalServerError, Code: "internal", Message: "Something went wrong on our side. Try again, and quote the request id if it keeps happening.", cause: cause}
}

// respondError writes err in the error envelope, logging what the caller is not told.
func respondError(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		apiErr = ErrInternal(err)
	}
	out := *apiErr
	out.RequestID = middleware.GetReqID(r.Context())
	if out.Status >= http.StatusInternalServerError {
		log.Error("request failed", "method", r.Method, "path", r.URL.Path, "request_id", out.RequestID, "error", err)
	}
	respondJSON(w, out.Status, errorEnvelope{Error: out})
}

// respondJSON writes v as the response body with status.
func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
