package application

import (
	"time"
)

// StorageProfileMode limits whether a configured profile can accept new writes.
type StorageProfileMode string

// Storage profile modes determine the permitted read and write operations.
const (
	StorageProfileModeReadWrite StorageProfileMode = "read_write"
	StorageProfileModeReadOnly  StorageProfileMode = "read_only"
	StorageProfileModeDisabled  StorageProfileMode = "disabled"
)

// StorageDeploymentProfile carries public immutable identity and configured access mode.
type StorageDeploymentProfile struct {
	StorageProfile
	Mode StorageProfileMode
}

// StorageEnvironment is the normalized deployment configuration for object storage.
// ConfigSHA256 excludes secret values and identifies this environment snapshot.
type StorageEnvironment struct {
	UploadPolicy                      UploadPolicy
	UploadSessionTTL                  time.Duration
	Revision                          string // Monotonically increasing deployment sequence.
	DefaultWriteProfile, ConfigSHA256 string
	Profiles                          []StorageDeploymentProfile
}
