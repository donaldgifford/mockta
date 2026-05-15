package handlers

import (
	"net/http"

	"github.com/donaldgifford/mockta/internal/oktaerr"
)

// Resetter is the subset of *store.Store the admin reset handler
// needs. Lets tests pass a fake.
type Resetter interface {
	Reset() error
}

// NewAdminReset returns a handler for `POST /admin/reset`. Used by
// the testcontainers cleanup hook to wipe state between test runs;
// not invoked from Terraform.
//
// Auth is enforced by the middleware chain the Server wires around
// this handler — see DESIGN-0001 Q6: /admin/reset requires the admin
// token, /health on the same port stays open.
func NewAdminReset(r Resetter) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := r.Reset(); err != nil {
			oktaerr.Write(w, http.StatusInternalServerError,
				oktaerr.CodeAPIValidationFailed,
				"reset failed: "+err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
