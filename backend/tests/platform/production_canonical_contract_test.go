package platform_test

import (
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

type productionCanonicalFixture struct {
	SuccessCases []struct {
		Name          string          `json:"name"`
		Input         json.RawMessage `json:"input"`
		CanonicalJSON string          `json:"canonical_json"`
		SHA256        string          `json:"sha256"`
	} `json:"success_cases"`
	FailureCases []struct {
		Name       string `json:"name"`
		RawJSON    string `json:"raw_json"`
		RawJSONHex string `json:"raw_json_hex"`
		ErrorCode  string `json:"error_code"`
	} `json:"failure_cases"`
}

func TestProductionCanonicalFixtureMatchesRFC8785AndFailureCodes(t *testing.T) {
	fixture := loadProductionCanonicalFixture(t)
	for _, value := range fixture.SuccessCases {
		t.Run(value.Name, func(t *testing.T) {
			encoded, err := canonical.JSON(value.Input)
			if err != nil || string(encoded) != value.CanonicalJSON {
				t.Fatalf("canonical JSON = %q, want %q, err=%v", encoded, value.CanonicalJSON, err)
			}
			hash, err := canonical.Hash(value.Input)
			if err != nil || hash != value.SHA256 {
				t.Fatalf("canonical hash = %q, want %q, err=%v", hash, value.SHA256, err)
			}
		})
	}
	for _, value := range fixture.FailureCases {
		t.Run(value.Name, func(t *testing.T) {
			raw := []byte(value.RawJSON)
			if value.RawJSONHex != "" {
				var err error
				raw, err = hex.DecodeString(value.RawJSONHex)
				if err != nil {
					t.Fatal(err)
				}
			}
			_, err := canonical.JSON(raw)
			if canonical.ErrorCode(err) != value.ErrorCode {
				t.Fatalf("canonical error = %v code=%q, want %q", err, canonical.ErrorCode(err), value.ErrorCode)
			}
		})
	}
}

func loadProductionCanonicalFixture(t *testing.T) productionCanonicalFixture {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve Production Canonical fixture path")
	}
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(current), "..", "fixtures", "agent", "production-canonical.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture productionCanonicalFixture
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	return fixture
}
