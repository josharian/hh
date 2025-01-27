package hh

import (
	"errors"
	"fmt"
	"io"
	"net/http"
)

// Error is an error type respected by Wrap.
// It controls the status code and text of the response.
type Error struct {
	StatusCode  int    // the HTTP status code to respond with
	Text        string // the text to respond with
	ContentType string // the content type to respond with, if blank, no content type is set
}

// Error returns the user-visible error message for e.
func (e *Error) Error() string {
	return fmt.Sprintf("%d: %v", e.StatusCode, e.Text)
}

// A JSONError indicates that a Jxxx API was unable to encode its data as JSON.
// This typically indicates a programmer error.
// As such, when a *JSONError is returned from Wrap,
// the client receives an HTTP 500 with default text.
type JSONError struct {
	Data any
	Err  error
}

func (e *JSONError) Error() string {
	return fmt.Sprintf("hh.Jnnn JSON encoding: %v (value: %#v)", e.Err, e.Data)
}

// A HandlerFunc is an http.HandlerFunc that returns an error. See Wrap.
type HandlerFunc func(http.ResponseWriter, *http.Request) error

// Wrap converts h to the standard http.HandlerFunc.
// All errors returned by h are passed through the errorware, in order.
// At that point, errors are converted to HTTP 500s (internal server error),
// unless they are of type *Error, in which case the error specifies the status code and text.
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
		if ee.ContentType != "" {
			w.Header().Set("Content-Type", ee.ContentType)
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
