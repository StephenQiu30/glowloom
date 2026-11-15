package postgres

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

var _ application.FrozenCompilationStore = (*FrozenInputStore)(nil)

func captureFrozenTemplate(tx *gorm.DB, request application.FreezeInputRequest, input *domain.FrozenInput) error {
	selection := request.TemplateSelection
	if selection == nil || input.CompilationInputs == nil {
		return domain.ErrFrozenInputInvalid
	}
	customizations := []domain.Customization{}
	if selection.Operation != "storyboard_video" {
		// Preferences belong to the account's personal home, even when the selected
		// project belongs to a team in another physical organization.
		var personal struct{ PhysicalScopeID uuid.UUID }
		read := tx.Raw(`SELECT physical_scope_id FROM workspace.creation_scope WHERE kind='personal' AND owner_user_id=? FOR SHARE`, request.ActorID).Scan(&personal)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected != 1 || personal.PhysicalScopeID == uuid.Nil {
			return application.ErrTemplateUnavailable
		}
		// Same lock as SaveCustomization, including the absent-row case. FOR SHARE
		// alone would not serialize a concurrent first preference INSERT.
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(69361,hashtext(?))`, personal.PhysicalScopeID.String()+"/"+request.ActorID.String()+"/"+selection.Operation).Error; err != nil {
			return err
		}
		read = tx.Raw(`SELECT id,operation,mode,content,base_template_id,revision,update_time FROM workspace.prompt_customization WHERE owner_id=? AND operation=? FOR SHARE`, request.ActorID, selection.Operation).Scan(&customizations)
		if read.Error != nil {
			return read.Error
		}
	}
	frozen, err := application.CaptureFrozenTemplate(*selection, customizations)
	if err != nil {
		return err
	}
	input.CompilationInputs.Template = &frozen
	return nil
}

func decodeFrozenCompilation(raw json.RawMessage, request application.CompileFrozenInputRequest) (application.FrozenInputCompilation, error) {
	var out application.FrozenInputCompilation
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&out) != nil || out.Key != request.Key || out.CatalogContext != request.CatalogContext {
		return out, domain.ErrFrozenInputConflict
	}
	contextSHA, err := request.CatalogContext.SHA256()
	if err != nil || out.ContextSHA256 != contextSHA || out.ModelVersionID != request.CatalogContext.ModelVersionID || out.Mode != request.CatalogContext.Mode || out.CompilerVersion != domain.CompilerVersion || out.CompilationSHA256 == "" || out.OutputCount < 1 || out.OutputCount > 8 || out.Media == nil || out.ReferenceMapping == nil {
		return out, domain.ErrFrozenInputConflict
	}
	canonical, err := writefence.CanonicalReceipt(raw)
	if err != nil {
		return out, domain.ErrFrozenInputConflict
	}
	expected, err := out.Receipt()
	if err != nil || !bytes.Equal(canonical, expected) {
		return out, domain.ErrFrozenInputConflict
	}
	// JSONB may return spaces inside RawMessage parameters. Restore the original
	// canonical nested bytes before returning the exact typed RPC receipt.
	if json.Unmarshal(expected, &out) != nil {
		return out, domain.ErrFrozenInputConflict
	}
	return out, nil
}

// ReadFrozenCompilation is immutable recovery, independent of today's source
// policy. Only an already committed exact command/result can be returned.
func (s *FrozenInputStore) ReadFrozenCompilation(ctx context.Context, request application.CompileFrozenInputRequest) (application.FrozenInputCompilation, error) {
	var out application.FrozenInputCompilation
	if err := s.ready(ctx); err != nil {
		return out, err
	}
	if _, err := request.CatalogContext.SHA256(); err != nil {
		return out, err
	}
	envelope, err := readFrozenKey(s.db.WithContext(ctx), request.Key, false)
	if err != nil {
		return out, err
	}
	command, err := application.FrozenCompilationCommand(envelope, request)
	if err != nil {
		return out, err
	}
	record, err := pgfence.New(s.db).Load(ctx, command.ID)
	if errors.Is(err, writefence.ErrNotFound) {
		return out, domain.ErrFrozenInputNotFound
	}
	if err != nil {
		return out, err
	}
	if !application.SameFrozenCommand(record.Command, command) {
		return out, domain.ErrFrozenInputConflict
	}
	if record.State == writefence.Frozen {
		return out, writefence.ErrFrozen
	}
	if record.State != writefence.Committed {
		return out, domain.ErrFrozenInputNotFound
	}
	return decodeFrozenCompilation(record.Receipt, request)
}

// CompileFrozenInput commits full result, Outbox and final writer receipt in one
// local transaction. All Identity/source proof IO precedes this method.
func (s *FrozenInputStore) CompileFrozenInput(ctx context.Context, request application.CompileFrozenInputRequest, command writefence.Command) (application.FrozenInputCompilation, error) {
	var out application.FrozenInputCompilation
	if err := s.ready(ctx); err != nil {
		return out, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		envelope, err := readFrozenKey(tx, request.Key, true)
		if err != nil {
			return err
		}
		expected, err := application.FrozenCompilationCommand(envelope, request)
		if err != nil {
			return err
		}
		if !application.SameFrozenCommand(expected, command) {
			return domain.ErrFrozenInputConflict
		}
		fences := pgfence.New(s.db)
		if err := fences.Guard(ctx, tx, command); err != nil {
			if !errors.Is(err, writefence.ErrCommitted) {
				return err
			}
			record, err := pgfence.New(tx).Load(ctx, command.ID)
			if err != nil {
				return err
			}
			out, err = decodeFrozenCompilation(record.Receipt, request)
			return err
		}
		if envelope.AdoptionBlocked {
			return domain.ErrFrozenInputBlocked
		}
		// Recheck and lock current rights/status without comparing mutable project
		// revisions or using current creative values for the frozen compilation.
		var current domain.FrozenInput
		if err := lockFrozenPolicy(tx, application.FreezeInputRequest{ActorID: envelope.Input.ActorID, ScopeID: envelope.Input.ScopeID, ProjectID: envelope.Input.ProjectID}, &current); err != nil {
			return err
		}
		if current.OrgID != envelope.Input.OrgID {
			return domain.ErrFrozenInputConflict
		}
		out, err = application.CompileFrozenSnapshot(envelope, request)
		if err != nil {
			return err
		}
		receipt, err := out.Receipt()
		if err != nil {
			return err
		}
		if err := tx.Exec(`INSERT INTO infra.outbox(id,topic,partition_key,payload) VALUES(?,?,?,?::jsonb)`, uuid.NewSHA1(request.Key.HandoffID, []byte("creation.frozen_input.compiled")), "glowloom.creation.frozen_input.compiled", envelope.Input.ProjectID.String(), string(receipt)).Error; err != nil {
			return err
		}
		return fences.Complete(ctx, tx, command, receipt)
	})
	return out, err
}
