// Package middleware holds HTTP middleware used by both listener
// muxes. Each middleware is a function from http.Handler to
// http.Handler so callers can compose them with the Chain helper or
// by hand.
package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/donaldgifford/mockta/internal/oktaerr"
)

const bearerPrefix = "Bearer "

// Auth returns middleware that checks for a Bearer token. Strict mode
// requires an exact-match against expected; permissive mode accepts
// any non-empty bearer. expected may be empty (meaning "no token
// configured"), in which case authentication is disabled — useful for
// quick scripts and tests.
//
// Comparison is constant-time so timing leaks can't be used to
// recover the token byte-by-byte. Not a real threat for a test mock,
// but it's the right shape and costs nothing.
func Auth(expected string, strict bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if expected == "" {
				next.ServeHTTP(w, r)
				return
			}
			tok, ok := bearerFromHeader(r.Header.Get("Authorization"))
			if !ok {
				oktaerr.Write(w, http.StatusUnauthorized,
					oktaerr.CodeMissingAuth,
					"Authentication required: missing Bearer token")
				return
			}
			if strict {
				if subtle.ConstantTimeCompare([]byte(tok), []byte(expected)) != 1 {
					oktaerr.Write(w, http.StatusUnauthorized,
						oktaerr.CodeInvalidToken,
						"Invalid bearer token")
					return
				}
			}
			// Permissive mode: any non-empty bearer passes.
			next.ServeHTTP(w, r)
		})
	}
}

// bearerFromHeader extracts the token from "Bearer <token>". Returns
// ("", false) if the header is empty or malformed.
func bearerFromHeader(h string) (string, bool) {
	if !strings.HasPrefix(h, bearerPrefix) {
		return "", false
	}
	tok := strings.TrimPrefix(h, bearerPrefix)
	if tok == "" {
		return "", false
	}
	return tok, true
}
