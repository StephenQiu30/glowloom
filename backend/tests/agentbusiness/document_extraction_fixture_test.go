package agentbusiness_test

import (
	"testing"

	script "github.com/StephenQiu30/glowloom-server/backend/creation/script/application"
	scriptwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/script"
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
