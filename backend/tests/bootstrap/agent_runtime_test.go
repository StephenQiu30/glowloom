package bootstrap_test

import (
	"strings"
	"testing"

	agentcontract "github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
	"github.com/StephenQiu30/lanverse/backend/internal/bootstrap"
	"github.com/StephenQiu30/lanverse/backend/internal/config"
)

func TestAPIRuntimeCatalogResolvesSharedStoryGraphBundle(t *testing.T) {
	configuration := config.Config{
		AgentURL: "http://agent:8787", AgentRuntimeImageDigest: "sha256:" + strings.Repeat("a", 64),
	}
	catalog, err := bootstrap.NewAgentRuntimeCatalog(configuration)
	if err != nil {
		t.Fatalf("default API runtime configuration must start: %v", err)
	}
	for _, bundleHash := range []string{agentcontract.StoryGraphSkillBundleHash, agentcontract.SceneAnalysisSkillBundleHash} {
		route, resolveErr := catalog.Resolve(bundleHash)
		if resolveErr != nil || route.BaseURL != configuration.AgentURL || route.ImageDigest != configuration.AgentRuntimeImageDigest {
			t.Fatalf("shared bundle route = %#v, error = %v", route, resolveErr)
		}
	}
	configuration.AgentRuntimeAdditionalRevisions = []config.AgentRuntimeRevision{{
		BundleHash: agentcontract.StoryGraphSkillBundleHash, BaseURL: "http://other-agent:8787",
		ImageDigest: "sha256:" + strings.Repeat("b", 64),
	}}
	if _, err = bootstrap.NewAgentRuntimeCatalog(configuration); err == nil {
		t.Fatal("conflicting configured bundle route must still be rejected")
	}
}
