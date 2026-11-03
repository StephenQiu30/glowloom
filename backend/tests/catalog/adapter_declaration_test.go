package catalog_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"

	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
)

func declarationPreset(t *testing.T) domain.PresetDocument {
	t.Helper()
	doc := domain.PresetDocument{SchemaVersion: 1, Name: "Restricted declaration", Config: domain.BuiltinPresets()[1].Config}
	doc.AdapterDeclaration = &domain.AdapterDeclaration{SchemaVersion: 1, AdapterKey: "openrouter", ProtocolID: "openrouter", BaseURL: doc.Config.BaseURL, Region: doc.Config.Region, Capabilities: []string{"text", "image", "video"}}
	return doc
}

func TestAdapterDeclarationStrictContract(t *testing.T) {
	doc := declarationPreset(t)
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	parsed, err := domain.ParsePreset(data)
	require.NoError(t, err)
	require.Equal(t, doc, parsed)
	for _, test := range []struct {
		name   string
		mutate func(*domain.AdapterDeclaration)
	}{
		{"version", func(d *domain.AdapterDeclaration) { d.SchemaVersion = 2 }},
		{"unknown protocol", func(d *domain.AdapterDeclaration) { d.ProtocolID = "arbitrary" }},
		{"wrong adapter protocol", func(d *domain.AdapterDeclaration) { d.ProtocolID = "openai" }},
		{"origin", func(d *domain.AdapterDeclaration) { d.BaseURL = "https://evil.invalid" }},
		{"region", func(d *domain.AdapterDeclaration) { d.Region = domain.RegionDomestic }},
		{"duplicate capability", func(d *domain.AdapterDeclaration) { d.Capabilities = []string{"text", "text"} }},
		{"mismatched capabilities", func(d *domain.AdapterDeclaration) { d.Capabilities = []string{"text"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			doc := declarationPreset(t)
			test.mutate(doc.AdapterDeclaration)
			require.ErrorIs(t, doc.Validate(), domain.ErrInvalidPreset)
		})
	}
	for _, extra := range []string{`"headers":{"Authorization":"sensitive-test-key"},`, `"credential_values":{"api_key":"sensitive-test-key"},`, `"script":"fetch()",`, `"protocol_id":"openai",`} {
		unsafe := strings.Replace(string(data), `"adapter_declaration":{`, `"adapter_declaration":{`+extra, 1)
		_, err := domain.ParsePreset([]byte(unsafe))
		require.ErrorIs(t, err, domain.ErrInvalidPreset)
		require.NotContains(t, err.Error(), "sensitive-test-key")
	}
}

func TestAdapterDeclarationAvailabilityRejectsBeforeSave(t *testing.T) {
	service := application.NewDeclaredProviderPresets(nil, func(string) bool { return false })
	doc := declarationPreset(t)
	actor := identityapp.Principal{ID: uuid.New(), OrgID: uuid.New(), PlatformRole: "platform_admin"}
	_, err := service.Save(t.Context(), actor, application.SavePresetInput{Slug: "restricted", RequestID: uuid.New(), Document: doc, PreviewHash: doc.Digest()})
	require.ErrorIs(t, err, domain.ErrInvalidPreset)
	require.NoError(t, application.NewDeclaredProviderPresets(nil, installedProtocol).ValidateDeclaration(doc))
	require.ErrorIs(t, application.NewProviderPresets(nil).ValidateDeclaration(doc), domain.ErrInvalidPreset)
}

func TestAdapterDeclarationDirectSaveJSONRejectsAmbiguousFields(t *testing.T) {
	for _, raw := range []string{
		`{"schema_version":1,"protocol_id":"openrouter","protocol_id":"openai"}`,
		`{"schema_version":1,"headers":{"Authorization":"secret-marker"}}`,
		`{"schema_version":1,"api_key":"secret-marker"}`,
		`{"schema_version":1,"PROTOCOL_ID":"openai"}`,
	} {
		var declaration domain.AdapterDeclaration
		err := json.Unmarshal([]byte(raw), &declaration)
		require.ErrorIs(t, err, domain.ErrInvalidPreset)
		require.NotContains(t, err.Error(), "secret-marker")
	}
}

func TestAdapterDeclarationDirectDocumentRejectsDuplicateDeclaration(t *testing.T) {
	doc := declarationPreset(t)
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	declaration, err := json.Marshal(doc.AdapterDeclaration)
	require.NoError(t, err)
	ambiguous := strings.TrimSuffix(string(data), "}") + `,"adapter_declaration":` + string(declaration) + `}`
	var decoded domain.PresetDocument
	require.ErrorIs(t, json.Unmarshal([]byte(ambiguous), &decoded), domain.ErrInvalidPreset)
	_, err = domain.ParsePreset([]byte(ambiguous))
	require.ErrorIs(t, err, domain.ErrInvalidPreset)
}
