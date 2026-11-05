package application

import (
	"context"

	"github.com/google/uuid"
)

// StorageProbeState is the persisted lifecycle of one exact-key diagnostic probe.
type StorageProbeState string

// Storage probe states record execution and verified outcomes.
const (
	StorageProbeRunning  StorageProbeState = "running"
	StorageProbeComplete StorageProbeState = "complete"
	StorageProbeCleaning StorageProbeState = "cleaning"
	StorageProbeCleaned  StorageProbeState = "cleaned"
)

// StorageProbeMode restricts diagnostics to the profile's configured access.
type StorageProbeMode = StorageProfileMode

// StorageProbeIntent is persisted before any diagnostic write. A missing final
// receipt leaves recoverable exact-key debt, rather than discarding ownership.
type StorageProbeIntent struct {
	ID                                                       uuid.UUID
	ProfileID, Bucket, ConfigSHA256, SentinelKey, RemovalKey string
	Mode                                                     StorageProbeMode
	State                                                    StorageProbeState
}

// StorageProbeTracker never derives diagnostic object targets from the route.
type StorageProbeTracker interface {
	RegisterProfiles(context.Context, []StorageProfile) error
	BeginStorageProbe(context.Context, StorageProbeIntent) error
	CompleteStorageProbe(context.Context, uuid.UUID, string) error
}

// StorageProbeCleanup claims only expired unsuccessful diagnostic runs.
type StorageProbeCleanup interface {
	RegisterProfiles(context.Context, []StorageProfile) error
	ClaimStorageProbeCleanup(context.Context, uuid.UUID) (StorageProbeIntent, error)
	CompleteStorageProbeCleanup(context.Context, uuid.UUID) error
}
