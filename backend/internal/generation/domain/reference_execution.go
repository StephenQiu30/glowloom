package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
	"github.com/google/uuid"
)

type GenerationContractRef struct {
	ContractID  string `json:"contract_id"`
	ContentHash string `json:"content_hash"`
}

func (ref GenerationContractRef) Valid() bool {
	return targetNamePattern.MatchString(ref.ContractID) && targetHashPattern.MatchString(ref.ContentHash)
}

type GenerationActionRef struct {
	ID          string `json:"id"`
	ContentHash string `json:"content_hash"`
}

func (ref GenerationActionRef) Valid() bool {
	return (GenerationRevisionRef{ID: ref.ID, Revision: 1, ContentHash: ref.ContentHash}).Valid()
}

type ReferenceCredentialRef struct {
	ID          string `json:"id"`
	Revision    int64  `json:"revision"`
	Fingerprint string `json:"fingerprint"`
}

type ReferenceExecutionReadSet struct {
	TargetRef            GenerationRevisionRef  `json:"generation_target_ref"`
	TargetReadSetRoot    string                 `json:"target_read_set_root"`
	ExpectedHeadRevision int64                  `json:"expected_head_revision"`
	BindingRef           GenerationRevisionRef  `json:"project_provider_binding_version_ref"`
	ConnectionRef        GenerationRevisionRef  `json:"provider_connection_version_ref"`
	CredentialRef        ReferenceCredentialRef `json:"provider_credential_version_ref"`
	ProfileRef           GenerationRevisionRef  `json:"provider_model_profile_version_ref"`
	RegistryReleaseHash  string                 `json:"adapter_registry_release_hash"`
	AdapterRef           GenerationContractRef  `json:"provider_adapter_contract_ref"`
	CompilerRef          GenerationContractRef  `json:"request_compiler_contract_ref"`
	CapabilityHash       string                 `json:"provider_capability_snapshot_hash"`
	ManifestHash         string                 `json:"compiled_request_manifest_hash"`
	OperationalPolicyRef GenerationContractRef  `json:"operational_limit_policy_ref"`
	AuthorizationRef     GenerationActionRef    `json:"reference_execution_authorization_ref"`
}

type InitialReferenceExecutionInput struct {
	ID          string                    `json:"execution_id"`
	WorkspaceID string                    `json:"workspace_id"`
	ProjectID   string                    `json:"project_id"`
	ReadSet     ReferenceExecutionReadSet `json:"read_set"`
	CreatedBy   string                    `json:"created_by"`
	CreatedAt   time.Time                 `json:"created_at"`
}

type ReferenceExecution struct {
	ContractID string `json:"contract_id"`
	Revision   int64  `json:"revision"`
	InitialReferenceExecutionInput
	ExecutionReadSetRoot string `json:"execution_read_set_root"`
	ContentHash          string `json:"content_hash"`
}

func BuildInitialReferenceExecution(input InitialReferenceExecutionInput) (ReferenceExecution, error) {
	for _, value := range []string{input.ID, input.WorkspaceID, input.ProjectID, input.CreatedBy} {
		id, err := uuid.Parse(value)
		if err != nil || id == uuid.Nil || id.String() != value {
			return ReferenceExecution{}, errors.New("invalid Reference execution identity")
		}
	}
	r := input.ReadSet
	for _, ref := range []GenerationRevisionRef{r.TargetRef, r.BindingRef, r.ConnectionRef, r.ProfileRef, {ID: r.CredentialRef.ID, Revision: r.CredentialRef.Revision, ContentHash: r.CredentialRef.Fingerprint}} {
		if !ref.Valid() {
			return ReferenceExecution{}, errors.New("invalid Reference execution version")
		}
	}
	for _, ref := range []GenerationContractRef{r.AdapterRef, r.CompilerRef, r.OperationalPolicyRef} {
		if !ref.Valid() {
			return ReferenceExecution{}, errors.New("invalid Reference execution contract")
		}
	}
	for _, hash := range []string{r.TargetReadSetRoot, r.RegistryReleaseHash, r.CapabilityHash, r.ManifestHash} {
		if !targetHashPattern.MatchString(hash) {
			return ReferenceExecution{}, errors.New("invalid Reference execution content identity")
		}
	}
	if !r.AuthorizationRef.Valid() || r.ExpectedHeadRevision != 0 || r.TargetRef.Revision != 1 || input.CreatedAt.IsZero() {
		return ReferenceExecution{}, errors.New("invalid initial Reference execution")
	}
	input.CreatedAt = input.CreatedAt.UTC().Truncate(time.Microsecond)
	raw, err := json.Marshal(r)
	if err != nil {
		return ReferenceExecution{}, err
	}
	root, err := canonical.Hash(raw)
	if err != nil {
		return ReferenceExecution{}, err
	}
	value := ReferenceExecution{ContractID: "reference-generation-execution", Revision: 1, InitialReferenceExecutionInput: input, ExecutionReadSetRoot: root}
	raw, err = json.Marshal(value)
	if err != nil {
		return ReferenceExecution{}, err
	}
	value.ContentHash, err = canonical.Hash(raw)
	if err != nil {
		return ReferenceExecution{}, err
	}
	return value, nil
}

func DecodeReferenceExecution(raw json.RawMessage) (ReferenceExecution, error) {
	var value ReferenceExecution
	if err := canonical.Decode(raw, &value); err != nil {
		return ReferenceExecution{}, err
	}
	rebuilt, err := BuildInitialReferenceExecution(value.InitialReferenceExecutionInput)
	if err != nil || !reflect.DeepEqual(value, rebuilt) {
		return ReferenceExecution{}, errors.New("Reference execution snapshot has drifted")
	}
	return value, nil
}

type ReferenceGenerationLimits struct {
	MaxBundles           int   `json:"max_bundles"`
	MaxSlots             int   `json:"max_slots"`
	MaxInputBytes        int64 `json:"max_input_bytes"`
	MaxOutputBytes       int64 `json:"max_output_bytes"`
	MaxConcurrentCalls   int   `json:"max_concurrent_calls"`
	SubmitTimeoutSeconds int   `json:"submit_timeout_seconds"`
	MaxDailyCalls        int   `json:"max_daily_calls"`
}

func DefaultReferenceGenerationLimits() ReferenceGenerationLimits {
	return ReferenceGenerationLimits{4, 4, 32 << 20, 10 << 20, 2, 180, 256}
}

func (limits ReferenceGenerationLimits) Ref() (GenerationContractRef, error) {
	if limits.MaxBundles < 1 || limits.MaxSlots < 1 || limits.MaxInputBytes < 1 || limits.MaxOutputBytes < 1 || limits.MaxConcurrentCalls < 1 || limits.SubmitTimeoutSeconds < 1 || limits.MaxDailyCalls < 1 {
		return GenerationContractRef{}, errors.New("invalid Reference generation limits")
	}
	raw, err := json.Marshal(limits)
	if err != nil {
		return GenerationContractRef{}, err
	}
	hash, err := canonical.Hash(raw)
	if err != nil {
		return GenerationContractRef{}, err
	}
	return GenerationContractRef{ContractID: "reference-generation-operational-limits", ContentHash: hash}, nil
}
