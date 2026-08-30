package httperr

import (
	"errors"
	"fmt"
	"net/http"
)

// ErrorDetail represents a specific field-level validation failure.
type ErrorDetail struct {
	Location string `json:"location,omitempty"`
	Message  string `json:"message"`
	Value    any    `json:"value,omitempty"`
}

// Problem represents an RFC 9457 compliant error payload.
type Problem struct {
	Type     string        `json:"type"`
	Title    string        `json:"title"`
	Status   int           `json:"status"`
	Detail   string        `json:"detail,omitempty"`
	Instance string        `json:"instance,omitempty"`
	Errors   []ErrorDetail `json:"errors,omitempty"`
}

// Error implements the standard Go error interface.
func (p *Problem) Error() string {
	if p.Detail != "" {
		return fmt.Sprintf("%s: %s (status %d)", p.Title, p.Detail, p.Status)
	}
	return fmt.Sprintf("%s (status %d)", p.Title, p.Status)
}

// GetStatus implements the huma.StatusError interface.
func (p *Problem) GetStatus() int {
	return p.Status
}

// NewNotFound creates a 404 Not Found Problem.
func NewNotFound(detail string) *Problem {
	return &Problem{
		Type:   "https://datatracker.ietf.org/doc/html/rfc9110#section-15.5.5",
		Title:  "Not Found",
		Status: http.StatusNotFound,
		Detail: detail,
	}
}

// NewConflict creates a 409 Conflict Problem.
func NewConflict(detail string) *Problem {
	return &Problem{
		Type:   "https://datatracker.ietf.org/doc/html/rfc9110#section-15.5.10",
		Title:  "Conflict",
		Status: http.StatusConflict,
		Detail: detail,
	}
}

// NewBadRequest creates a 400 Bad Request Problem.
func NewBadRequest(detail string) *Problem {
	return &Problem{
		Type:   "https://datatracker.ietf.org/doc/html/rfc9110#section-15.5.1",
		Title:  "Bad Request",
		Status: http.StatusBadRequest,
		Detail: detail,
	}
}

// NewValidation creates a 422 Unprocessable Entity Problem with field details.
func NewValidation(detail string, errs ...ErrorDetail) *Problem {
	return &Problem{
		Type:   "https://datatracker.ietf.org/doc/html/rfc9110#section-15.5.21",
		Title:  "Unprocessable Entity",
		Status: http.StatusUnprocessableEntity,
		Detail: detail,
		Errors: errs,
	}
}

// NewUnauthorized creates a 401 Unauthorized Problem.
func NewUnauthorized(detail string) *Problem {
	return &Problem{
		Type:   "https://datatracker.ietf.org/doc/html/rfc9110#section-15.5.2",
		Title:  "Unauthorized",
		Status: http.StatusUnauthorized,
		Detail: detail,
	}
}

// NewForbidden creates a 403 Forbidden Problem.
func NewForbidden(detail string) *Problem {
	return &Problem{
		Type:   "https://datatracker.ietf.org/doc/html/rfc9110#section-15.5.4",
		Title:  "Forbidden",
		Status: http.StatusForbidden,
		Detail: detail,
	}
}

// NewInternal creates a 500 Internal Server Error Problem.
func NewInternal(detail string) *Problem {
	return &Problem{
		Type:   "https://datatracker.ietf.org/doc/html/rfc9110#section-15.6.1",
		Title:  "Internal Server Error",
		Status: http.StatusInternalServerError,
		Detail: detail,
	}
}

// Transform converts a generic error into an RFC 9457 Problem.
func Transform(err error) *Problem {
	if err == nil {
		return nil
	}

	var p *Problem
	if errors.As(err, &p) {
		return p
	}

	return NewInternal("An unexpected error occurred")
}
