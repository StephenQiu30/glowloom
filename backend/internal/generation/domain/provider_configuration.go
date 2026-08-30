package domain

import "time"

const (
	MediaProviderVolcengine = "volcengine_ark"
	MediaProviderOpenAI     = "openai"
	MediaProviderGoogle     = "google_gemini"

	MediaModalityImage = "image"
	MediaModalityVideo = "video"

	ProviderPurposeReferenceAsset = "reference_asset"
	ProviderPurposeShotFrame      = "shot_frame"
	ProviderPurposeShotVideo      = "shot_video"

	ProviderStateEnabled  = "enabled"
	ProviderStateDisabled = "disabled"

	ProviderCipherAES256GCM = "aes-256-gcm"
)

type MediaPresetField struct {
	Key       string
	Type      string
	Required  bool
	WriteOnly bool
	Pattern   string
	Minimum   int64
	Maximum   int64
}

type MediaConnectionPreset struct {
	PresetKey              string
	PresetVersion          int64
	ProviderKey            string
	DisplayName            string
	Description            string
	ProviderHomeURL        string
	AdapterContractVersion string
	FixedConfig            map[string]any
	EditableFields         []MediaPresetField
	CredentialFields       []MediaPresetField
	SupportedFactoryModes  []string
}

type MediaModelPreset struct {
	PresetKey                string
	PresetVersion            int64
	ProviderKey              string
	DisplayName              string
	Family                   string
	Modality                 string
	ExternalModelID          string
	AdapterContractVersion   string
	AdapterTransportContract string
	CapabilitySchemaVersion  string
	BillingMetric            string
	FixedDefaults            map[string]any
	EditableOverrides        []MediaPresetField
}

type MediaPresets struct {
	Connections []MediaConnectionPreset
	Models      []MediaModelPreset
}

type ProviderSecretContext struct {
	WorkspaceID, ProviderKey, CredentialID string
	Revision                               int64
	KeyID                                  string
}

type EncryptedProviderSecret struct {
	CipherSuite string
	KeyID       string
	Nonce       []byte
	Ciphertext  []byte
	Fingerprint string
}

type ProviderCredentialVersion struct {
	ID, WorkspaceID, ConnectionKey, ProviderKey string
	Revision                                    int64
	CipherSuite, KeyID                          string
	Nonce, Ciphertext                           []byte
	SecretFingerprint                           string
	CreatedBy                                   string
	CreatedAt                                   time.Time
}

type ProviderConnectionVersion struct {
	ID, WorkspaceID, ConnectionKey string
	Revision                       int64
	SourcePresetKey                string
	SourcePresetVersion            int64
	PresetSnapshotHash             string
	ProviderKey, DisplayName       string
	CredentialVersionID            string
	ResolvedConfig                 map[string]any
	State, AdapterContractVersion  string
	ContentHash, CreatedBy         string
	CreatedAt                      time.Time
}

type ProviderModelProfileVersion struct {
	ID, WorkspaceID, ProfileKey, ConnectionKey string
	Revision                                   int64
	CreationSource                             map[string]any
	ProviderKey, ExternalModelID               string
	Modality, Family                           string
	AdapterTransportContract                   string
	CapabilitySchemaVersion                    string
	BillingMetric                              string
	Defaults                                   map[string]any
	State, ContentHash, CreatedBy              string
	CreatedAt                                  time.Time
}

type ProjectProviderBindingVersion struct {
	ID, WorkspaceID, ProjectID, Purpose string
	Revision                            int64
	ConnectionVersionID                 string
	CredentialVersionID                 string
	ModelProfileVersionID               string
	ProviderKey, Modality               string
	AdapterContractVersion              string
	ContentHash, CreatedBy              string
	CreatedAt                           time.Time
}
