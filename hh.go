package hh

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Error is an error type respected by Wrap.
// It controls the status code and text of the response.
type Error struct {
	StatusCode int    // the HTTP status code to respond with
	Text       string // the text to respond with
}

// Error returns an internally-facing error message for e.
func (e *Error) Error() string {
	return fmt.Sprintf("%d: %v", e.StatusCode, e.Text)
}

// E returns an HTTP error with status statusCode, with the default status text.
func E(statusCode int) error {
	return &Error{StatusCode: statusCode, Text: http.StatusText(statusCode)}
}

// S returns an HTTP error with status statusCode and text s.
func S(statusCode int, s string) error {
	return &Error{StatusCode: statusCode, Text: s}
}

// F returns an HTTP error with status statusCode and Sprintf-formatted text.
func F(statusCode int, format string, args ...any) error {
	return &Error{StatusCode: statusCode, Text: fmt.Sprintf(format, args...)}
}

// J returns an HTTP error with status statusCode, accompanied by data encoded as JSON.
// If data cannot be JSON-encoded, the response to the client
// will be an HTTP 500 (Internal Server Error) with default status text.
// The error will contain details of the encoding failure.
func J(statusCode int, data any) error {
	buf, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("hh.J encoding failed: %w (value: %#v)", err, data)
	}
	return &Error{StatusCode: statusCode, Text: string(buf)}
}

var (
	ErrBadRequest          = E(http.StatusBadRequest)
	ErrUnauthorized        = E(http.StatusUnauthorized)
	ErrMethodNotAllowed    = E(http.StatusMethodNotAllowed)
	ErrNotFound            = E(http.StatusNotFound)
	ErrTooManyRequests     = E(http.StatusTooManyRequests)
	ErrInternalServerError = E(http.StatusInternalServerError)
	ErrServiceUnavailable  = E(http.StatusServiceUnavailable)
)

// A HandlerFunc is an http.HandlerFunc that returns an error. See Wrap.
type HandlerFunc func(http.ResponseWriter, *http.Request) error

// Wrap converts h to the standard http.HandlerFunc.
// All errors returned by h are passed through the errorware, in order.
// At that point, errors are converted to HTTP 500s (internal server error),
// unless they are of type *Error, in which case the error specifies the status code and text.
//
// Wrap does not buffer output.
// If you write to the response writer and then return an error,
// returning the HTTP status code header will fail, and
// the error text will be sent after the response writer output.
func Wrap(h HandlerFunc, errorware ...func(*http.Request, error) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := h(w, r)
		for _, fn := range errorware {
			err = fn(r, err)
		}
		if err == nil {
			return
		}
		var ee *Error
		if !errors.As(err, &ee) {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(ee.StatusCode)
		io.WriteString(w, ee.Text)
	}
}

// Mux is a convenience wrapper around http.ServeMux.
// Calls to Handle register a HandlerFunc with the wrapped mux.
type Mux struct {
	*http.ServeMux
	errorware []func(*http.Request, error) error
}

// WrapMux creates a new Mux wrapping mux.
// It applies errorware to each wrapped handler during registration.
func WrapMux(mux *http.ServeMux, errorware ...func(*http.Request, error) error) *Mux {
	return &Mux{ServeMux: mux, errorware: errorware}
}

// HandleWrap registers handler for pattern.
func (m *Mux) HandleWrap(pattern string, handler HandlerFunc) {
	m.ServeMux.Handle(pattern, Wrap(handler, m.errorware...))
}
