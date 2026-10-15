package application

import (
	"time"

	"github.com/google/uuid"
)

// StorageDeploymentProfile carries public immutable identity and proposed access mode.
type StorageDeploymentProfile struct {
	StorageProfile
	Mode string
}

// StorageActivation binds a whole validated configuration to one explicit CAS.
// ConfigSHA256 excludes secret values; runtime reports and probes bind that digest.
type StorageActivation struct {
	UploadPolicy                                                  UploadPolicy
	UploadSessionTTL                                              time.Duration
	ExpectedRevision, Revision, DefaultWriteProfile, ConfigSHA256 string
	Profiles                                                      []StorageDeploymentProfile
}

// StorageRuntime is an actual live process's configuration/capability lease.
// Reporting a capability requires the corresponding production wiring to exist.
type StorageRuntime struct {
	InstanceID                   uuid.UUID
	Role, Revision, ConfigSHA256 string
	Capabilities                 []string
}
