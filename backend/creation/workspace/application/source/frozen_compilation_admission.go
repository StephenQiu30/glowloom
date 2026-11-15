package source

import (
	"context"
	"errors"

	prompt "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// FrozenInputCompilationAdmission keeps original Identity proof and current source
// policy outside the result transaction, whose final writer guard remains local.
type FrozenInputCompilationAdmission struct {
	source       FrozenInputSourceAdmission
	writer       FrozenInputWriter
	inputs       prompt.FrozenInputStore
	compilations prompt.FrozenCompilationStore
}

// NewFrozenInputCompilationAdmission injects the same authenticated proof pipeline
// as Freeze. A private service token cannot replace the original user admission.
func NewFrozenInputCompilationAdmission(source FrozenInputSourceAdmission, writer FrozenInputWriter, inputs prompt.FrozenInputStore, compilations prompt.FrozenCompilationStore) *FrozenInputCompilationAdmission {
	return &FrozenInputCompilationAdmission{source, writer, inputs, compilations}
}

// Compile returns a committed original result before requesting any fresh policy.
// First compilation resolves the original consumer proof without retargeting it.
func (s *FrozenInputCompilationAdmission) Compile(ctx context.Context, request prompt.CompileFrozenInputRequest) (prompt.FrozenInputCompilation, error) {
	if s == nil || s.source == nil || s.writer == nil || s.inputs == nil || s.compilations == nil {
		return prompt.FrozenInputCompilation{}, writefence.ErrUnavailable
	}
	if _, err := request.CatalogContext.SHA256(); err != nil {
		return prompt.FrozenInputCompilation{}, err
	}
	original, err := s.compilations.ReadFrozenCompilation(ctx, request)
	if err == nil {
		return original, nil
	}
	if !errors.Is(err, domain.ErrFrozenInputNotFound) {
		return prompt.FrozenInputCompilation{}, err
	}
	envelope, err := s.inputs.ReadFrozenInput(ctx, request.Key)
	if err != nil {
		return prompt.FrozenInputCompilation{}, err
	}
	if envelope.AdoptionBlocked {
		return prompt.FrozenInputCompilation{}, domain.ErrFrozenInputBlocked
	}
	command, err := prompt.FrozenCompilationCommand(envelope, request)
	if err != nil {
		return prompt.FrozenInputCompilation{}, err
	}
	permit, err := s.source.Admit(ctx, AdmissionRequest{Command: envelope.Input.Command, Target: writefence.WriterExecution, ScopeID: envelope.Input.ScopeID, ProjectID: envelope.Input.ProjectID})
	if err != nil {
		return prompt.FrozenInputCompilation{}, err
	}
	if !prompt.SameFrozenCommand(permit.Command, envelope.Input.Command) || permit.Target != writefence.WriterExecution || permit.AccountID != envelope.Input.ActorID || permit.SessionID != envelope.Input.SessionID || permit.ScopeID != envelope.Input.ScopeID || permit.ProjectID != envelope.Input.ProjectID || permit.PhysicalScopeID != envelope.Input.OrgID {
		return prompt.FrozenInputCompilation{}, domain.ErrFrozenInputConflict
	}
	record, err := s.writer.Admit(ctx, command)
	if err != nil {
		return prompt.FrozenInputCompilation{}, err
	}
	if !prompt.SameFrozenCommand(record.Command, command) {
		return prompt.FrozenInputCompilation{}, domain.ErrFrozenInputConflict
	}
	if record.State == writefence.Frozen {
		return prompt.FrozenInputCompilation{}, writefence.ErrFrozen
	}
	return s.compilations.CompileFrozenInput(ctx, request, command)
}
