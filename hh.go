package hh

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// An HTTPResponseError is an error that specifies an HTTP status code and text to return to a client.
type HTTPResponseError interface {
	error
	HTTPStatusCode() int
	HTTPStatusText() string
}

// ResponseError is a convenience type that implements HTTPResponseError.
type ResponseError struct {
	StatusCode int    // the HTTP status code to respond with
	StatusText string // the text that accompanies the status code
}

func (e *ResponseError) Error() string {
	return fmt.Sprintf("%d: %v", e.StatusCode, e.StatusText)
}
func (e *ResponseError) HTTPStatusCode() int    { return e.StatusCode }
func (e *ResponseError) HTTPStatusText() string { return e.StatusText }

// Error returns an HTTP error with status statusCode, with the default status text.
func Error(statusCode int) error {
	return &ResponseError{StatusCode: statusCode, StatusText: http.StatusText(statusCode)}
}

// ErrorText returns an HTTP error with status statusCode and text s.
func ErrorText(statusCode int, s string) error {
	return &ResponseError{StatusCode: statusCode, StatusText: s}
}

// Errorf returns an HTTP error with status statusCode and Sprintf-formatted text.
func Errorf(statusCode int, format string, args ...any) error {
	return &ResponseError{StatusCode: statusCode, StatusText: fmt.Sprintf(format, args...)}
}

// ErrorJSON returns an HTTP error with status statusCode, accompanied by data encoded as JSON.
// If data cannot be JSON-encoded, the response to the client
// will be an HTTP 500 (Internal Server Error) with default 500 status text.
// The error will contain details of the encoding failure.
func ErrorJSON(statusCode int, data any) error {
	buf, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("hh.ErrorJSON: encoding failed: %w (value: %#v)", err, data)
	}
	return &ResponseError{StatusCode: statusCode, StatusText: string(buf)}
}

var (
	ErrBadRequest          = Error(http.StatusBadRequest)
	ErrUnauthorized        = Error(http.StatusUnauthorized)
	ErrMethodNotAllowed    = Error(http.StatusMethodNotAllowed)
	ErrNotFound            = Error(http.StatusNotFound)
	ErrTooManyRequests     = Error(http.StatusTooManyRequests)
	ErrInternalServerError = Error(http.StatusInternalServerError)
	ErrServiceUnavailable  = Error(http.StatusServiceUnavailable)
)

// A HandlerFunc is an http.HandlerFunc that returns an error. See Wrap.
type HandlerFunc func(http.ResponseWriter, *http.Request) error

// Wrap converts h to the standard http.HandlerFunc.
// All errors returned by h are passed through the errorware, in order.
// At that point, errors are converted to HTTP 500s (internal server error),
// unless they implement HTTPResponseError, in which case the error specifies the status code and text.
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
		re, ok := err.(HTTPResponseError)
		if !ok {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(re.HTTPStatusCode())
		io.WriteString(w, re.HTTPStatusText())
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
