// Package oktaerr writes Okta-shaped error responses.
//
// Real Okta surfaces failures via a stable JSON envelope. The
// terraform provider parses this shape, so any mockta error must
// match it byte-for-byte modulo the errorId, which we prefix with
// `mockta-` so failures are obvious in provider logs.
package oktaerr

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
)

// Common Okta error codes used by mockta. The real Okta error code
// catalog has hundreds of entries; v0 uses the small subset that the
// provider's happy-path handlers actually surface.
const (
	CodeAPIValidationFailed = "E0000001"
	CodeResourceNotFound    = "E0000007"
	CodeMissingAuth         = "E0000011"
	CodeInvalidToken        = "E0000020"
	CodeResourceConflict    = "E0000038"
	CodeUnsupportedFeature  = "E0000041"
)

// Response is the JSON-encoded error envelope.
type Response struct {
	Code    string  `json:"errorCode"`
	Summary string  `json:"errorSummary"`
	Link    string  `json:"errorLink"`
	ID      string  `json:"errorId"`
	Causes  []Cause `json:"errorCauses"`
}

// Cause holds one nested validation detail.
type Cause struct {
	Summary string `json:"errorSummary"`
}

// marshal is overridable so the marshal-failure fallthrough can be
// exercised by tests. Production code uses json.Marshal directly; the
// indirection costs one function-pointer dereference per error
// response — negligible on the error path.
var marshal = json.Marshal

// Write serializes resp at the given HTTP status with the
// application/json content type. Causes is allowed to be nil — it
// serializes as an empty array, matching real Okta.
func Write(w http.ResponseWriter, status int, code, summary string, causes ...Cause) {
	if causes == nil {
		causes = []Cause{}
	}
	resp := Response{
		Code:    code,
		Summary: summary,
		Link:    code,
		ID:      newErrorID(),
		Causes:  causes,
	}
	body, err := marshal(resp)
	if err != nil {
		// Marshal failure is unreachable for a static struct, but
		// fall through to a plain-text 500 if it ever happens.
		slog.Default().Error("marshal okta error envelope", "err", err)
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintln(w, "internal: error envelope marshal failure")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(body)
}

// newErrorID returns a `mockta-<hex>` ID. Real Okta uses a longer
// opaque ID; we keep the same shape and prefix with `mockta-` so a
// failing provider log line tells the developer the error came from
// mockta rather than real Okta.
func newErrorID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("mockta-%x", b)
}
