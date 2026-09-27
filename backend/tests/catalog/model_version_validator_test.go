package catalog_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/catalog/adapter/paramvalidation"
)

func TestModelVersionValidatorUsesSharedParamSchemaCases(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "internal", "catalog", "testdata", "param-schema", "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name        string          `json:"name"`
		Valid       bool            `json:"valid"`
		Modes       []string        `json:"modes"`
		Limits      json.RawMessage `json:"limits"`
		ParamSchema json.RawMessage `json:"param_schema"`
	}
	if err := json.Unmarshal(content, &cases); err != nil || len(cases) < 10 {
		t.Fatalf("decode shared validation cases: %v (%d cases)", err, len(cases))
	}
	validator, err := paramvalidation.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			version := validModelVersion(uuid.New())
			version.Modes = tc.Modes
			version.Limits = tc.Limits
			version.ParamSchema = tc.ParamSchema
			err := validator.Validate(version)
			if tc.Valid && err != nil {
				t.Fatalf("valid model version rejected: %v", err)
			}
			if !tc.Valid && !errors.Is(err, paramvalidation.ErrInvalidConfiguration) {
				t.Fatalf("invalid model version accepted: %v", err)
			}
		})
	}
}
