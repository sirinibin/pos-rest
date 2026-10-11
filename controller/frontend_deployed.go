package controller

import (
	"encoding/json"
	"net/http"

	"github.com/sirinibin/startpos/backend/models"
)

// FrontendDeployed : handler for POST /v1/app/frontend-deployed
//
// Called by the frontend deploy script once the site serves the new build.
// Every open page is told over the WebSocket and pages with auto refresh on
// check index.html once. No sign-in: it carries no data and only makes pages
// re-read index.html, at most once per models.FrontendDeployedMinGap.
func FrontendDeployed(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]bool{"announced": models.NotifyFrontendDeployed()}) //nolint:errcheck
}
