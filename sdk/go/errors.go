package liteboxd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

var (
	// ErrNotFound is returned when a resource is not found.
	ErrNotFound = &APIError{StatusCode: 404, Message: "resource not found"}

	// ErrConflict is returned when a resource already exists.
	ErrConflict = &APIError{StatusCode: 409, Message: "resource already exists"}

	// ErrBadRequest is returned when the request is invalid.
	ErrBadRequest = &APIError{StatusCode: 400, Message: "invalid request"}

	// ErrInternal is returned when an internal server error occurs.
	ErrInternal = &APIError{StatusCode: 500, Message: "internal server error"}

	// ErrUnauthorized is returned when authentication fails.
	ErrUnauthorized = &APIError{StatusCode: 401, Message: "unauthorized"}
)

// APIError represents an error response from the API.
type APIError struct {
	StatusCode int
	Message    string
	Err        error
}

// Error returns the error message.
func (e *APIError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Err)
	}
	return e.Message
}

// Is checks if the error matches target.
func (e *APIError) Is(target error) bool {
	t, ok := target.(*APIError)
	if !ok {
		return false
	}
	return e.StatusCode == t.StatusCode
}

// Unwrap returns the underlying error.
func (e *APIError) Unwrap() error {
	return e.Err
}

// errorResponse represents an error response from the API.
type errorResponse struct {
	Error string `json:"error"`
}

// handleErrorResponse handles an error response from the API.
func handleErrorResponse(resp *http.Response) error {
	// Read body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    http.StatusText(resp.StatusCode),
			Err:        err,
		}
	}

	// Try to decode error response
	var errResp errorResponse
	if err := json.Unmarshal(body, &errResp); err == nil && errResp.Error != "" {
		return &APIError{
			StatusCode: resp.StatusCode,
			Message:    errResp.Error,
		}
	}

	// Return status text if no error message in body
	return &APIError{
		StatusCode: resp.StatusCode,
		Message:    http.StatusText(resp.StatusCode),
	}
}

// IsNotFound checks if an error is a not found error.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsConflict checks if an error is a conflict error.
func IsConflict(err error) bool {
	return errors.Is(err, ErrConflict)
}

// IsBadRequest checks if an error is a bad request error.
func IsBadRequest(err error) bool {
	return errors.Is(err, ErrBadRequest)
}
