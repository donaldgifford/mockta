package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/donaldgifford/mockta/internal/oktaerr"
	"github.com/donaldgifford/mockta/internal/store"
)

// decodeJSONBodyLenient parses r.Body into v, accepting unknown
// fields. Used for resources where the provider sometimes sends
// computed/legacy fields we don't validate but also shouldn't reject
// (Q7 of IMPL-0001 — accept-and-ignore unrecognized fields). Returns
// a parsed-OK boolean; on failure the 400 envelope is already written
// and the caller should bail.
func decodeJSONBodyLenient(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		oktaerr.Write(w, http.StatusBadRequest,
			oktaerr.CodeAPIValidationFailed,
			"Api validation failed: "+err.Error())
		return false
	}
	return true
}

// writeStoreError maps a store error to the right HTTP status +
// envelope. Callers should return immediately after invoking.
func writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		oktaerr.Write(w, http.StatusNotFound,
			oktaerr.CodeResourceNotFound,
			"Not found: "+err.Error())
	case errors.Is(err, store.ErrConflict):
		oktaerr.Write(w, http.StatusConflict,
			oktaerr.CodeResourceConflict,
			"Conflict: "+err.Error())
	default:
		oktaerr.Write(w, http.StatusInternalServerError,
			oktaerr.CodeAPIValidationFailed,
			"Internal error: "+err.Error())
	}
}

// loginEmailLooksValid is the strict-mode validation for a user
// login. Per IMPL-0001 Q7 we keep this RFC-5322-*ish* — Okta accepts
// non-RFC-5322 strings as login (e.g. with `_` and `+`) so we use a
// lighter check: must have one `@`, at least one char on each side.
func loginEmailLooksValid(s string) bool {
	at := strings.Index(s, "@")
	return at > 0 && at < len(s)-1 && !strings.Contains(s[at+1:], "@")
}
