package application

import (
	"context"
	"encoding/hex"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
)

// ReferencePinConsumptionKey identifies Media's one original Creation consumer.
// Media supplies existing pin facts; reading never adopts or removes a reference.
type ReferencePinConsumptionKey struct {
	PinID, AcquireCommandID, SourceCommandID uuid.UUID
	HandoffID, ScopeID, ProjectID, AssetID   uuid.UUID
	SourceCommandSHA256, PinSnapshotSHA256   string
	Kind                                     string
	AssetRevision                            int64
}

// Validate rejects ambiguous source identities and incomplete immutable facts.
func (k ReferencePinConsumptionKey) Validate() error {
	if k.PinID == uuid.Nil || k.AcquireCommandID == uuid.Nil || k.SourceCommandID == uuid.Nil || k.HandoffID == uuid.Nil ||
		k.ScopeID == uuid.Nil || k.ProjectID == uuid.Nil || k.AssetID == uuid.Nil || k.AssetRevision < 1 || k.Kind == "" ||
		k.SourceCommandID != uuid.NewSHA1(k.HandoffID, []byte("creation.frozen_input.freeze")) {
		return domain.ErrFrozenInputInvalid
	}
	for _, value := range []string{k.SourceCommandSHA256, k.PinSnapshotSHA256} {
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != 32 || hex.EncodeToString(decoded) != value {
			return domain.ErrFrozenInputInvalid
		}
	}
	return nil
}

// ReferencePinConsumption is permanent evidence about the original source effect.
// A blocked or cancelled snapshot still retains its historical references. Only
// a command permanently frozen before any snapshot can prove non-adoption here.
type ReferencePinConsumption struct {
	Key                  ReferencePinConsumptionKey
	Outcome              string
	SourceSnapshotSHA256 string
}

// ReferencePinConsumptionReader reads committed adoption or permanent non-adoption.
// A missing command, pending command, timeout or missing row alone is not proof.
type ReferencePinConsumptionReader interface {
	ReadReferencePinConsumption(context.Context, ReferencePinConsumptionKey) (ReferencePinConsumption, error)
}
