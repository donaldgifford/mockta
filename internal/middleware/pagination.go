package middleware

import (
	"fmt"
	"net/http"
)

// EmitNextLink writes a `Link: <url>; rel="next"` header for the
// given request, mirroring Okta's pagination semantics. v0 always
// emits an empty cursor — the next page is always empty — to
// exercise the provider's pagination code path without actually
// paging.
//
// This is a helper, not middleware, because pagination only applies
// to specific list endpoints and the cursor depends on per-handler
// state. Handlers call it explicitly when emitting list responses.
func EmitNextLink(w http.ResponseWriter, r *http.Request, cursor string) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	q := r.URL.Query()
	q.Set("after", cursor)
	link := fmt.Sprintf(`<%s://%s%s?%s>; rel="next"`,
		scheme, r.Host, r.URL.Path, q.Encode())
	w.Header().Add("Link", link)
}
