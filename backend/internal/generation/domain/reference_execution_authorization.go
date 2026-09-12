package domain

import (
	"encoding/json"
	"errors"
	"reflect"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/canonical"
)

// GenerationRevisionRef identifies an immutable Generation-owned version.
// The containing contract supplies its workspace, project and fact type.
type GenerationRevisionRef struct {
	ID          string `json:"id"`
	Revision    int64  `json:"revision"`
	ContentHash string `json:"content_hash"`
}

func (ref GenerationRevisionRef) Valid() bool {
	id, err := uuid.Parse(ref.ID)
	return err == nil && id != uuid.Nil && id.String() == ref.ID && ref.Revision >= 1 && ref.Revision <= 9007199254740991 && targetHashPattern.MatchString(ref.ContentHash)
}

type InitialReferenceExecutionAuthorizationInput struct {
	WorkspaceID                              string                `json:"workspace_id"`
	ProjectID                                string                `json:"project_id"`
	GenerationTargetRef                      GenerationRevisionRef `json:"generation_target_ref"`
	SelectedProjectProviderBindingVersionRef GenerationRevisionRef `json:"selected_project_provider_binding_version_ref"`
	HumanActionRef                           string                `json:"human_action_ref"`
	MembershipTokenVersion                   int                   `json:"membership_token_version"`
	AuthorizedBy                             string                `json:"authorized_by"`
	AuthorizedAt                             time.Time             `json:"authorized_at"`
}

type ReferenceExecutionAuthorization struct {
	ContractID string `json:"contract_id"`
	Kind       string `json:"kind"`
	InitialReferenceExecutionAuthorizationInput
	PreviousExecutionRef           *GenerationRevisionRef `json:"previous_execution_ref"`
	UnresolvedCallAcknowledgements []json.RawMessage      `json:"unresolved_call_acknowledgements"`
	ReasonCode                     string                 `json:"reason_code"`
	ContentHash                    string                 `json:"content_hash"`
}

func BuildInitialReferenceExecutionAuthorization(input InitialReferenceExecutionAuthorizationInput) (ReferenceExecutionAuthorization, error) {
	for _, value := range []string{input.WorkspaceID, input.ProjectID, input.HumanActionRef, input.AuthorizedBy} {
		id, err := uuid.Parse(value)
		if err != nil || id == uuid.Nil || id.String() != value {
			return ReferenceExecutionAuthorization{}, errors.New("invalid Reference execution authorization identity")
		}
	}
	if !input.GenerationTargetRef.Valid() || !input.SelectedProjectProviderBindingVersionRef.Valid() || input.MembershipTokenVersion < 1 || input.AuthorizedAt.IsZero() {
		return ReferenceExecutionAuthorization{}, errors.New("invalid initial Reference execution authorization")
	}
	input.AuthorizedAt = input.AuthorizedAt.UTC().Truncate(time.Microsecond)
	value := ReferenceExecutionAuthorization{ContractID: "reference-execution-authorization-production", Kind: "initial_execution", InitialReferenceExecutionAuthorizationInput: input, UnresolvedCallAcknowledgements: []json.RawMessage{}, ReasonCode: "initial_execution"}
	raw, err := json.Marshal(value)
	if err != nil {
		return ReferenceExecutionAuthorization{}, err
	}
	value.ContentHash, err = canonical.Hash(raw)
	return value, err
}

func DecodeReferenceExecutionAuthorization(raw json.RawMessage) (ReferenceExecutionAuthorization, error) {
	var value ReferenceExecutionAuthorization
	if err := canonical.Decode(raw, &value); err != nil {
		return ReferenceExecutionAuthorization{}, err
	}
	rebuilt, err := BuildInitialReferenceExecutionAuthorization(value.InitialReferenceExecutionAuthorizationInput)
	if err != nil || !reflect.DeepEqual(value, rebuilt) {
		return ReferenceExecutionAuthorization{}, errors.New("Reference execution authorization has drifted")
	}
	return value, nil
}
