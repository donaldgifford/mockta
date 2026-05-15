package handlers

import (
	"encoding/json"
	"net/http"
)

// healthBody is precomputed at package init so the handler is
// alloc-free per request. The structure is static; marshal can't
// fail.
var healthBody = mustMarshalHealth()

func mustMarshalHealth() []byte {
	b, err := json.Marshal(map[string]string{"status": "ok"})
	if err != nil {
		panic("internal: marshal static health body: " + err.Error())
	}
	return b
}

// NewHealth returns a handler for `GET /health`. It's unauthenticated
// and lives on the admin port (:9090). Docker's HEALTHCHECK and
// terraform's docker_container.healthcheck both poll it. v0 reports
// 200 unconditionally once the server is up; future tiers can grow
// real liveness/readiness probes.
func NewHealth() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(healthBody)
	})
}
