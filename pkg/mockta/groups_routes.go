//go:build !mockta_v0_undersized

package mockta

import (
	"net/http"

	"github.com/donaldgifford/mockta/internal/handlers"
	"github.com/donaldgifford/mockta/internal/store"
)

// wireGroupRoutes is the production wiring for the /api/v1/groups
// surface. The undersized variant (groups_routes_undersized.go) is
// gated on `mockta_v0_undersized` and replaces this with a no-op
// stub so all /api/v1/groups* requests 501 via the catch-all — the
// gap-list determinism golden fixture relies on that behavior.
func wireGroupRoutes(mux *http.ServeMux, st *store.Store) {
	mux.Handle("POST /api/v1/groups", handlers.NewGroupsCreate(st))
	mux.Handle("GET /api/v1/groups", handlers.NewGroupsList(st))
	mux.Handle("GET /api/v1/groups/{id}", handlers.NewGroupsGet(st))
	mux.Handle("PUT /api/v1/groups/{id}", handlers.NewGroupsUpdate(st))
	mux.Handle("DELETE /api/v1/groups/{id}", handlers.NewGroupsDelete(st))
	mux.Handle("PUT /api/v1/groups/{gid}/users/{uid}",
		handlers.NewGroupMembershipAdd(st))
	mux.Handle("DELETE /api/v1/groups/{gid}/users/{uid}",
		handlers.NewGroupMembershipRemove(st))
	mux.Handle("GET /api/v1/groups/{gid}/users",
		handlers.NewGroupMembershipList(st))
}
