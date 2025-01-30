WARNING: Package hh is experimental while I tinker with the API.

---

Package hh contains HTTP handler helpers.

```sh
go get github.com/josharian/hh@latest
```

Full discussion below. But most people just want examples, so...

# Examples

Basic mux setup. Note that you can mix-and-match wrapped and unwrapped handlers.

```go
mux := hh.WrapMux(http.NewServeMux())
mux.HandleFunc("GET /{$}", srv.handleRoot) // normal stdlib handler
mux.HandleWrap("GET /thing/{id}", srv.handleThing) // wrapped handler
```

Basic handler usage, showing off lots of different error return options.

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
    json.NewEncoder(w).Write(thing)
    return nil
}
```

Adding in errorware:

```go
func slogHTTPErrors(r *http.Request, err error) error {
	if err != nil {
		slog.ErrorContext(r.Context(), "http request failed", "method", r.Method, "url", r.URL.String(), "err", err)
	}
	return err
}

// All wrapped HTTP handlers will have their errors (nil or otherwise!) passed through slogHTTPErrors.
mux := hh.WrapMux(http.NewServeMux(), slogHTTPErrors)
```

Use for a single handler, without touching the mux:

```go
subrouter.HandleFunc("/foo", hh.Wrap(srv.handleFoo)) // can also pass errorware to Wrap
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

The secondary goals are minimality, compositionality, and "batteries included" APIs for core, common functionality.

## Non-goals

Anything that can be left out! This includes middleware, improved routing, etc.

It is easy to add APIs later, but painful to remove them.

If there's clear consensus on common errorware, they might be a candidate.

## Approach

There are three parts to the package.

1. Adapters for HTTP handlers that return errors.
2. Special errors to trigger common HTTP responses.
3. Hooks for "errorware": Inspecting and modifying errors on the way out.

These are all fundamentally interwoven, which is why they're all lumped together in one package.

The adapters are `func Wrap`, which is the core low level adapter, and `type Mux`, which lets you avoid writing out `hh.Wrap` repeatedly, particularly when you're re-using a standard suite of errorware.

The core special error interface is `HTTPResponseError`, which gives total control over the HTTP response. There are helpers for the most common uses:

* `Error` responds with the default text for the error code.
* `ErrorText` responds with fixed text.
* `Errorf` responds with fmt.Sprintf-formatted text.
* `ErrorJSON` responds with JSON-encoded information.

And a set of top level `Err*` errors for the most common errors (as determined by some highly scientific grepping).

Both `Wrap` and `Mux` support integrated errorware, which is a way to log, inspect, and replace errors after the HTTP handler has finished processing.

# License

MIT

# Contributing

Contributions are welcome, but I prioritize my life over open source maintainership. Plan accordingly.

# Acknowledgements

Thank you to [Jonathan Hall](https://jhall.io) for excellent API design feedback.
