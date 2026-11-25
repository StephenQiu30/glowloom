package agentbusiness_test

import (
	"testing"

	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
	script "github.com/StephenQiu30/glowloom-server/backend/internal/creation/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/config"
)

func referenceDocumentExtractor(t *testing.T) script.SourceExtractor {
	t.Helper()
	extractor, err := scriptwire.ProvideSourceExtractor(config.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return extractor
}
