package middleware

import "net/http"

// Func is the shape every middleware in this package conforms to.
// Naming it lets callers spell out chains without repeating the
// signature.
type Func func(http.Handler) http.Handler

// Chain composes middleware around a handler. The first middleware in
// the slice is the outermost (runs first); the last is innermost
// (runs last before the handler).
func Chain(h http.Handler, mws ...Func) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
