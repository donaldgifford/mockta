package middleware

import (
	"net/http"
	"time"

	"github.com/donaldgifford/mockta/internal/store"
)

// AuditSink is the subset of *store.Store the audit middleware
// needs. Defining it here lets tests swap a fake without spinning
// up a real Store.
type AuditSink interface {
	AppendAudit(e *store.AuditEntry) error
}

// statusRecorder wraps http.ResponseWriter to capture the status code
// and gap ID for the audit log.
type statusRecorder struct {
	http.ResponseWriter
	status int
	gapID  string
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// GapHeader is the response header handlers set when serving a 501
// gap. The audit middleware reads it and forwards into the audit log.
// Using a header is cleaner than a context-value handoff because it
// composes with the standard http.Handler interface.
const GapHeader = "X-Mockta-Gap"

func (r *statusRecorder) Header() http.Header { return r.ResponseWriter.Header() }

// Audit logs every request to the given sink. The recorded entry
// captures method, path, status, and the gap ID (if the handler set
// the X-Mockta-Gap response header). Append errors are dropped so a
// failing audit log doesn't fail the request.
func Audit(sink AuditSink) Func {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rec, r)

			// Read the gap ID set by the handler (if any) and strip
			// the header before the response leaves the proxy chain.
			rec.gapID = rec.Header().Get(GapHeader)
			if rec.gapID != "" {
				rec.Header().Del(GapHeader)
			}

			// A failed audit write must not fail the request — the
			// audit log is observability, not a hard dependency.
			//nolint:errcheck // see comment above
			sink.AppendAudit(&store.AuditEntry{
				TS:     time.Now().UTC(),
				Method: r.Method,
				Path:   r.URL.Path,
				Status: rec.status,
				GapID:  rec.gapID,
			})
		})
	}
}
