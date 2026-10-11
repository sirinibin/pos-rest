package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirinibin/startpos/backend/models"
)

func postFrontendDeployed(t *testing.T) (int, bool) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/app/frontend-deployed", nil)
	w := httptest.NewRecorder()
	FrontendDeployed(w, req)
	var out struct {
		Announced bool `json:"announced"`
	}
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return w.Code, out.Announced
}

// The deploy script calls it without a token: it must be accepted, and it is
// harmless because announcements are limited to one per gap.
func TestFrontendDeployed_Unauthenticated(t *testing.T) {
	old := models.FrontendDeployedMinGap
	t.Cleanup(func() { models.FrontendDeployedMinGap = old })

	models.FrontendDeployedMinGap = 0
	if code, announced := postFrontendDeployed(t); code != http.StatusAccepted || !announced {
		t.Fatalf("first call: %d announced=%v, want 202 announced", code, announced)
	}

	models.FrontendDeployedMinGap = time.Hour
	if code, announced := postFrontendDeployed(t); code != http.StatusAccepted || announced {
		t.Fatalf("repeat within the gap: %d announced=%v, want 202 not announced", code, announced)
	}
}
