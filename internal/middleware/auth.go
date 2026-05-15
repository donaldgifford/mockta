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

// tokenPrefixes lists the auth scheme prefixes mockta accepts on the
// Authorization header. Real Okta uses `SSWS <token>` for API tokens;
// `Bearer <token>` is the generic form most tooling expects. We
// accept both so the okta terraform provider (SSWS) and curl-style
// scripts (Bearer) both work without configuration.
var tokenPrefixes = []string{"SSWS ", "Bearer "}

// Auth returns middleware that checks for a bearer/SSWS token.
// Strict mode requires an exact-match against expected; permissive
// mode accepts any non-empty token. expected may be empty (meaning
// "no token configured"), in which case authentication is disabled —
// useful for quick scripts and tests.
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
			tok, ok := tokenFromHeader(r.Header.Get("Authorization"))
			if !ok {
				oktaerr.Write(w, http.StatusUnauthorized,
					oktaerr.CodeMissingAuth,
					"Authentication required: missing SSWS or Bearer token")
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

// tokenFromHeader extracts the token from the Authorization header,
// matching any of the accepted schemes in tokenPrefixes. Returns
// ("", false) if no scheme matches or the trimmed token is empty.
func tokenFromHeader(h string) (string, bool) {
	for _, prefix := range tokenPrefixes {
		if !strings.HasPrefix(h, prefix) {
			continue
		}
		tok := strings.TrimPrefix(h, prefix)
		if tok == "" {
			return "", false
		}
		return tok, true
	}
	return "", false
}
