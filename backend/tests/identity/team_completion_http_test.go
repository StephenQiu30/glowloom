package identity_test

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	identityhttp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/http"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	workspacehttp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/http"
	workspace "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

func TestGovernanceLargeRevisionWire(t *testing.T) {
	const revision int64 = 9007199254740993
	for _, value := range []any{identity.Invitation{Revision: revision}, identity.AuthorityProposal{Revision: revision}, workspace.PlatformTeam{Revision: revision}} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), `"revision":"9007199254740993"`) {
			t.Fatalf("lossy revision: %s", raw)
		}
	}
	for _, value := range []any{&identityhttp.GovernanceMutation{}, &identityhttp.GovernanceVersion{}, &identityhttp.AuthorityProposalRequest{}, &workspacehttp.TeamProfileRequest{}, &workspacehttp.TeamLifecycleRequest{}} {
		if err := json.Unmarshal([]byte(`{"expected_revision":"9007199254740993"}`), value); err != nil {
			t.Fatal(err)
		}
		for _, raw := range []string{`{"expected_revision":9007199254740993}`, `{"expected_revision":"9223372036854775808"}`, `{"expected_revision":"1.5"}`} {
			if err := json.Unmarshal([]byte(raw), value); err == nil {
				t.Fatalf("invalid version decoded: %s", raw)
			}
		}
	}
}
func TestGovernanceStrictQueryBoundary(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api")
	identityhttp.NewGovernanceHandler(nil).Register(g)
	identityhttp.NewInvitationHandler(nil).Register(g)
	workspacehttp.NewPlatformTeamHandler(nil).Register(g)
	for _, path := range []string{"/api/platform/admin-grants?limit=1&limit=2", "/api/platform/account-invitations?offset=1", "/api/me/authority-proposals?limit=0", "/api/platform/teams?governance_blocked=1", "/api/platform/teams?limit=1&limit=2"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), "GET", path, nil))
		if w.Code != 422 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("query boundary %s: %d", path, w.Code)
		}
	}
}
