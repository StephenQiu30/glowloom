package postgres

import (
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

var _ application.ReferencePinConsumptionReader = (*FrozenInputStore)(nil)

// ReadReferencePinConsumption reads the original source decision without minting
// an admission. Completed and permanently frozen commands are separate proofs;
// pending commands and missing snapshots alone cannot release a Media pin.
func (s *FrozenInputStore) ReadReferencePinConsumption(ctx context.Context, key application.ReferencePinConsumptionKey) (application.ReferencePinConsumption, error) {
	out := application.ReferencePinConsumption{Key: key}
	if err := s.ready(ctx); err != nil {
		return out, err
	}
	if err := key.Validate(); err != nil {
		return out, err
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		original, err := pgfence.New(tx).Load(ctx, key.SourceCommandID)
		if err != nil {
			return err
		}
		if original.Command.Action != "creation.frozen_input.freeze" || original.Command.SHA256 != key.SourceCommandSHA256 || !pinCommandResources(original.Command, key) {
			return domain.ErrFrozenInputConflict
		}
		row, rowErr := readFrozenRow(tx, key.HandoffID, false)
		switch original.State {
		case writefence.Frozen:
			if len(original.Receipt) != 0 {
				return domain.ErrFrozenInputConflict
			}
			if !errors.Is(rowErr, domain.ErrFrozenInputNotFound) {
				if rowErr != nil {
					return rowErr
				}
				return domain.ErrFrozenInputConflict
			}
			out.Outcome = "not_adopted_frozen"
			return nil
		case writefence.Committed:
			if rowErr != nil {
				return rowErr
			}
			envelope, err := frozenEnvelope(row)
			if err != nil {
				return err
			}
			input := envelope.Input
			var receipt struct {
				HandoffID      uuid.UUID `json:"handoff_id"`
				RequestID      uuid.UUID `json:"request_id"`
				SnapshotSHA256 string    `json:"snapshot_sha256"`
			}
			if json.Unmarshal(original.Receipt, &receipt) != nil || receipt.HandoffID != input.HandoffID || receipt.RequestID != input.RequestID || receipt.SnapshotSHA256 != envelope.SnapshotSHA256 {
				return domain.ErrFrozenInputConflict
			}
			if !application.SameFrozenCommand(input.SourceCommand, original.Command) || input.ScopeID != key.ScopeID || input.ProjectID != key.ProjectID {
				return domain.ErrFrozenInputConflict
			}
			for _, pin := range input.Pins {
				if pin.PinID == key.PinID && pin.AcquireCommandID == key.AcquireCommandID && pin.AssetID == key.AssetID &&
					pin.AssetRevision == key.AssetRevision && pin.Kind == key.Kind && pin.SnapshotSHA256 == key.PinSnapshotSHA256 {
					out.Outcome, out.SourceSnapshotSHA256 = "adopted", envelope.SnapshotSHA256
					return nil
				}
			}
			return domain.ErrFrozenInputConflict
		default:
			return domain.ErrReferenceUnavailable
		}
	})
	if err != nil {
		return application.ReferencePinConsumption{}, err
	}
	return out, nil
}

func pinCommandResources(command writefence.Command, key application.ReferencePinConsumptionKey) bool {
	if _, err := command.AuthorityDigest(); err != nil || len(command.Authorities) != 4 {
		return false
	}
	seen := map[string]bool{}
	for _, authority := range command.Authorities {
		name := string(authority.Issuer) + "/" + string(authority.Kind)
		if seen[name] {
			return false
		}
		seen[name] = true
		switch name {
		case "identity/account", "identity/session":
		case "creation/scope":
			if authority.ID != key.ScopeID {
				return false
			}
		case "creation/project":
			if authority.ID != key.ProjectID {
				return false
			}
		default:
			return false
		}
	}
	return seen["identity/account"] && seen["identity/session"] && seen["creation/scope"] && seen["creation/project"]
}
