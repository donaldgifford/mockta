// Package handlers contains the per-endpoint HTTP handlers. Each
// handler is constructed via a New* function that takes its
// dependencies (store, config) and returns an http.Handler; the
// Server in pkg/mockta wires them onto the router.
package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/donaldgifford/mockta/internal/store"
)

// orgResponse is a trimmed Okta `/api/v1/org` payload — only the
// fields the provider reads for connectivity validation. Real Okta
// returns far more, but the provider doesn't inspect the rest.
type orgResponse struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Subdomain  string    `json:"subdomain"`
	Status     string    `json:"status"`
	WebSite    string    `json:"website"`
	Created    time.Time `json:"created"`
	LastUpdate time.Time `json:"lastUpdated"`
}

// NewOrg returns a handler for `GET /api/v1/org`. The provider hits
// this on every plan to validate connectivity, so it must succeed
// unconditionally regardless of org state.
func NewOrg(orgName string) http.Handler {
	created := time.Unix(0, 0).UTC()
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := orgResponse{
			ID:         store.NewID("org", orgName),
			Name:       orgName,
			Subdomain:  orgName,
			Status:     "ACTIVE",
			WebSite:    "https://" + orgName + ".mockta.local",
			Created:    created,
			LastUpdate: created,
		}
		writeJSON(w, resp)
	})
}

// writeJSON is a tiny helper to serialize and emit a 200 JSON
// response. On marshal failure we fall through to a 500 with no body
// — handler responses are static struct types, so marshal failures
// are unreachable in practice. Resource handlers that need a non-200
// status set it manually before calling writeJSON.
func writeJSON(w http.ResponseWriter, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(body)
}
