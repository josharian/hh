WARNING: Package hh is experimental while I tinker with the API.

---

Package hh provides support for HTTP handlers that return errors.

```sh
go get github.com/josharian/hh@latest
```

Full discussion below. But most people just want usage examples, so...

# Examples

Basic usage. Note that you can mix-and-match wrapped and unwrapped handlers.

```go
mux.HandleFunc("GET /{$}", srv.handleRoot)                  // normal stdlib handler
mux.HandleFunc("GET /thing/{id}", hh.Wrap(srv.handleThing)) // wrapped handler
```

Basic handler, showing off lots of different error return options.

```go
func (s *HTTPServer) handleThing(w http.ResponseWriter, r *http.Request) error {
	// This is toy code. Don't nitpick or copy unthinkingly, please!
	id := r.PathValue("id")
	thing, err := s.DB.LookUpThing(id)
	if errors.Is(err, sql.ErrNoRows) {
		return hh.ErrNotFound // returns a 404, with text "Not Found"
	}
	if err != nil {
		return err // returns a 500, with text "Internal Server Error"
	}
	authorized := s.checkAuth(r, thing)
	if !authorized {
		return hh.ErrorText(http.StatusUnauthorized, "no thing for you") // returns 401, with custom text
	}
	if msg := thing.isBroken(); msg != nil {
		return hh.Errorf(http.StatusServiceUnavailable, "this this is temporarily broken: %v", msg)
	}
	nextAllowedRequest := ratelimitDelay(r)
	if delay := nextAllowedRequest.Sub(time.Now()); delay > 0 {
		info := map[string]float64{"sleep_sec": delay.Seconds()}
		return hh.ErrorJSON(http.StatusTooManyRequests, info) // return a 429 with JSON-encoded info
	}
	// OK!
	json.NewEncoder(w).Write(thing)
	return nil
}
```

Adding in errorware:

```go
func slogHTTP(r *http.Request, err error) error {
	if err == nil {
		slog.DebugContext(r.Context(), "http request", "method", r.Method, "url", r.URL.String())
	} else {
		slog.ErrorContext(r.Context(), "http request failed", "method", r.Method, "url", r.URL.String(), "err", err)
	}
	return err
}

// all handleThing errors (nil or otherwise) will be passed through slogHTTP
mux.HandleFunc("GET /thing/{id}", hh.Wrap(srv.handleThing, slogHTTP)) // all handleThing errors
```

If this gets repetitive, use a closure:

```go
wrap := func(fn hh.HandlerFunc) http.HandlerFunc {
	return hh.Wrap(fn, slogHTTP)
}
```

# What is it

## Goals

The primary goal is to make it easy to write HTTP handlers that return errors. That helps avoid this dreaded mistake:

```go
authorized := checkAuth(r)
if !authorized {
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}
// do sensitive stuff
```

The secondary goals are minimality, compositionality, and just enough "batteries included" APIs for extremely common functionality.

## Non-goals

Anything that can be left out! This includes middleware, improved routing, etc.

It is easy to add APIs later, but painful to remove them.

If there's clear consensus on common errorware, they might be a candidate.

## Approach

There are three parts to the package.

1. An adapter for HTTP handlers that return errors (`Wrap`).
2. Special errors for common HTTP responses.
3. Hooks for "errorware": Inspecting, modifying, replacing, or removing errors on the way out.

These are all fundamentally interwoven, which is why they're all lumped together in one package.

### Wrap

The `Wrap` adapter converts handlers with errors to handlers. It buffers all responses written by wrapped handlers. This ensures that returning an error at any point is safe to do, because no output will have been written to the client. If buffering is undesirable for a particular endpoint, do not use hh for that endpoint.

### Errors and responses

The core special error interface is `HTTPResponseError`, which gives total control over the HTTP response. There is also a basic implementation, `ResponseError`, which supports sending a particular HTTP status code and text. (For more control over responses, such as setting Content-Type headers, create your own implementation to suit your needs.)

There are helpers for the most common uses, implemented using `ResponseError`:

* `Error` responds with the default text for the error code.
* `ErrorText` responds with fixed text and an error code.
* `Errorf` responds with fmt.Sprintf-formatted text.
* `ErrorJSON` responds with JSON-encoded information.

And a set of top level `Err*` errors for the most common errors (as determined by some highly scientific grepping).

### Errorware

`Wrap` supports integrated errorware, which is a way to log, inspect, and replace errors after the HTTP handler has finished processing.

# License

MIT

# Contributing

Contributions are welcome, but I prioritize my life over open source maintainership. Plan accordingly.

# Acknowledgements

Thank you to [Jonathan Hall](https://jhall.io), [Joe Tsai](https://github.com/dsnet), and [David Crawshaw](https://crawshaw.io) for excellent API design feedback.
