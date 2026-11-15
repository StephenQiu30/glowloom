package source

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	prompt "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// FrozenInputSourceAdmission resolves the consumer's original Identity admission
// and records current Creation resource policy for the same command and target.
type FrozenInputSourceAdmission interface {
	Admit(context.Context, AdmissionRequest) (Permit, error)
}

// FrozenInputWriter admits only the independent source responsibility command.
// Persistence owns its final Guard, snapshot/task/Outbox and receipt transaction.
type FrozenInputWriter interface {
	Admit(context.Context, writefence.Command) (writefence.Record, error)
}

// FrozenInputAdmission connects original consumer policy to frozen source
// persistence. Source and consumer retain distinct identities and exact receipts.
type FrozenInputAdmission struct {
	source FrozenInputSourceAdmission
	writer FrozenInputWriter
	inputs prompt.FrozenInputStore
}

// NewFrozenInputAdmission injects independent source, writer and business stores.
func NewFrozenInputAdmission(source FrozenInputSourceAdmission, writer FrozenInputWriter, inputs prompt.FrozenInputStore) *FrozenInputAdmission {
	return &FrozenInputAdmission{source: source, writer: writer, inputs: inputs}
}

// Freeze uses the consumer's existing Identity->Creation admission at target
// Execution. The separate source command binds the exact selection and consumer
// intent; it cannot be presented as another Identity admission or consumer effect.
// All Identity RPC and Media pin verification occur outside SQL locks.
func (s *FrozenInputAdmission) Freeze(ctx context.Context, request prompt.FreezeInputRequest) (prompt.FrozenInputEnvelope, error) {
	if s == nil || s.source == nil || s.writer == nil || s.inputs == nil {
		return prompt.FrozenInputEnvelope{}, writefence.ErrUnavailable
	}
	if request.Command.Action != "execution.frozen_input.accept" || request.SourceCommand.Action != "creation.frozen_input.freeze" || request.HandoffID == uuid.Nil || request.SourceCommand.ID != uuid.NewSHA1(request.HandoffID, []byte("creation.frozen_input.freeze")) || request.SourceCommand.ID == request.Command.ID || domain.ValidateFrozenAuthorities(request.Command, request.ActorID, request.SessionID, request.ScopeID, request.ProjectID) != nil || domain.ValidateFrozenAuthorities(request.SourceCommand, request.ActorID, request.SessionID, request.ScopeID, request.ProjectID) != nil {
		return prompt.FrozenInputEnvelope{}, domain.ErrFrozenInputInvalid
	}
	consumerAuthorities, _ := request.Command.AuthorityDigest()
	sourceAuthorities, _ := request.SourceCommand.AuthorityDigest()
	if consumerAuthorities != sourceAuthorities {
		return prompt.FrozenInputEnvelope{}, writefence.ErrConflict
	}
	digest, err := request.SourceCommandSHA256()
	if err != nil {
		return prompt.FrozenInputEnvelope{}, err
	}
	if request.SourceCommand.SHA256 != digest {
		return prompt.FrozenInputEnvelope{}, writefence.ErrConflict
	}
	permit, err := s.source.Admit(ctx, AdmissionRequest{Command: request.Command, Target: writefence.WriterExecution, ScopeID: request.ScopeID, ProjectID: request.ProjectID})
	if err != nil {
		if errors.Is(err, ErrScopeNotFound) || errors.Is(err, ErrProjectNotFound) {
			return prompt.FrozenInputEnvelope{}, domain.ErrFrozenInputNotFound
		}
		return prompt.FrozenInputEnvelope{}, fmt.Errorf("authorize frozen input consumer intent: %w", err)
	}
	if !prompt.SameFrozenCommand(permit.Command, request.Command) || permit.Target != writefence.WriterExecution || permit.AccountID != request.ActorID || permit.SessionID != request.SessionID || permit.ScopeID != request.ScopeID || permit.ProjectID != request.ProjectID || permit.PhysicalScopeID == uuid.Nil {
		return prompt.FrozenInputEnvelope{}, writefence.ErrConflict
	}
	record, err := s.writer.Admit(ctx, request.SourceCommand)
	if err != nil {
		return prompt.FrozenInputEnvelope{}, fmt.Errorf("admit frozen input source responsibility: %w", err)
	}
	if !prompt.SameFrozenCommand(record.Command, request.SourceCommand) {
		return prompt.FrozenInputEnvelope{}, writefence.ErrConflict
	}
	if record.State == writefence.Frozen {
		return prompt.FrozenInputEnvelope{}, writefence.ErrFrozen
	}
	if record.State != writefence.Admitted && record.State != writefence.Committed {
		return prompt.FrozenInputEnvelope{}, writefence.ErrConflict
	}
	envelope, err := s.inputs.FreezeInput(ctx, request)
	if err != nil {
		return prompt.FrozenInputEnvelope{}, err
	}
	if envelope.Input.OrgID != permit.PhysicalScopeID {
		return prompt.FrozenInputEnvelope{}, writefence.ErrConflict
	}
	return envelope, nil
}
