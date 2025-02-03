package hh

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

// An HTTPResponseError is an error that can render itself as an HTTP response.
type HTTPResponseError interface {
	error
	RenderHTTP(w http.ResponseWriter)
}

// ResponseError is a convenience type that implements HTTPResponseError.
type ResponseError struct {
	StatusCode int    // the HTTP status code to respond with
	StatusText string // the text that accompanies the status code
}

var _ HTTPResponseError = (*ResponseError)(nil)

func (e *ResponseError) Error() string {
	return fmt.Sprintf("%d: %v", e.StatusCode, e.StatusText)
}

func (e *ResponseError) RenderHTTP(w http.ResponseWriter) {
	http.Error(w, e.StatusText, e.StatusCode)
}

// Error returns a ResponseError with status statusCode, with the default status text.
func Error(statusCode int) error {
	return &ResponseError{StatusCode: statusCode, StatusText: http.StatusText(statusCode)}
}

// ErrorText returns a ResponseError with status statusCode and text s.
func ErrorText(statusCode int, s string) error {
	return &ResponseError{StatusCode: statusCode, StatusText: s}
}

// Errorf returns a ResponseError with status statusCode and Sprintf-formatted text.
func Errorf(statusCode int, format string, args ...any) error {
	return &ResponseError{StatusCode: statusCode, StatusText: fmt.Sprintf(format, args...)}
}

// ErrorJSON returns a ResponseError with status statusCode, accompanied by data encoded as JSON.
// If data cannot be JSON-encoded, ErrorJSON returns an error created with fmt.Errorf.
// In this case, the response to the client will be an HTTP 500 (Internal Server Error)
// with default 500 status text, and the error will contain details of the encoding failure.
// ErrorJSON does not set the Content-Type header.
// To do that, implement a custom HTTPResponseError.
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

// Wrap converts h to a standard http.HandlerFunc.
//
// All errors returned by h are passed through the errorware, in order.
// After errorware has been applied, non-nil errors are converted to HTTP 500s (internal server error),
// unless they implement HTTPResponseError, or wrap an error that does,
// in which case the error renders the response.
//
// Wrap buffers output and response headers until h returns.
// This ensures that errors are correctly sent to the client.
// For this reason, a wrapped handler's http.ResponseWriter
// does not implement http.Flusher or http.Hijacker.
// If this is not acceptable, do not use Wrap for this handler.
// This package is designed to allow mix-and-match with non-error-returning handlers.
func Wrap(h HandlerFunc, errorware ...func(*http.Request, error) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		bufw := new(bufferingResponseWriter)
		err := h(bufw, r)
		if bufw.err != nil {
			if err != nil {
				err = fmt.Errorf("response write error (%v) after handler error: %w", bufw.err, err)
			} else {
				err = bufw.err
			}
		}
		for _, fn := range errorware {
			err = fn(r, err)
		}
		if err == nil {
			bufw.flush(w)
			return
		}

		re := asHTTPResponseError(err)
		if re == nil {
			// not an HTTPResponseError, convert to 500
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		re.RenderHTTP(w)
	}
}

func asHTTPResponseError(err error) HTTPResponseError {
	for {
		if hre, ok := err.(HTTPResponseError); ok {
			return hre
		}
		switch x := err.(type) {
		case interface{ Unwrap() error }:
			err = x.Unwrap()
			if err == nil {
				return nil
			}
			// continue outer for loop
		case interface{ Unwrap() []error }:
			for _, err := range x.Unwrap() {
				if hre := asHTTPResponseError(err); hre != nil {
					return hre
				}
			}
			return nil
		default:
			return nil
		}
	}
}

type bufferingResponseWriter struct {
	header    http.Header
	buffer    bytes.Buffer
	code      int
	wroteCode bool
	wroteBody bool
	err       error // Accumulate response writing errors
}

func (w *bufferingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	if w.wroteBody {
		w.setError(ErrorText(http.StatusInternalServerError, "headers modified after being sent"))
		// Still return a copy to prevent undefined behavior
		h := make(http.Header, len(w.header))
		for k, v := range w.header {
			h[k] = v
		}
		return h
	}
	return w.header
}

func (w *bufferingResponseWriter) Write(b []byte) (int, error) {
	if !w.wroteCode {
		w.WriteHeader(http.StatusOK)
	}
	w.wroteBody = true
	return w.buffer.Write(b)
}

func (w *bufferingResponseWriter) WriteHeader(code int) {
	if w.wroteCode {
		w.setError(ErrorText(http.StatusInternalServerError, "multiple calls to WriteHeader"))
		return
	}
	if w.wroteBody {
		w.setError(ErrorText(http.StatusInternalServerError, "WriteHeader called after Write"))
		return
	}
	w.code = code
	w.wroteCode = true
}

func (w *bufferingResponseWriter) setError(err error) {
	if w.err == nil {
		w.err = err
	}
}

func (w *bufferingResponseWriter) flush(dst http.ResponseWriter) {
	for k, v := range w.header {
		dst.Header()[k] = v
	}
	if w.wroteCode {
		dst.WriteHeader(w.code)
	}
	if w.buffer.Len() > 0 {
		// intentionally ignore errors
		// there's little we can do about them
		_, _ = dst.Write(w.buffer.Bytes())
	}
}
