//go:build mockta_v0_undersized

package mockta

import (
	"net/http"

	"github.com/donaldgifford/mockta/internal/store"
)

// wireGroupRoutes is the undersized-build stub. With the
// `mockta_v0_undersized` tag set, every /api/v1/groups* request falls
// through to the 501 catch-all so the gap-list determinism fixture
// observes a stable sequence of MOCKTA_GAP_* IDs. The published image
// is *never* built with this tag — see Phase 7 of IMPL-0001.
func wireGroupRoutes(_ *http.ServeMux, _ *store.Store) {}
