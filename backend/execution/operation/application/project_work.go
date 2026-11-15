package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// ProjectWorkRequest identifies Creation's closed admission gate and one complete
// page of original registered commands. It grants no execution or funding rights.
type ProjectWorkRequest struct {
	CloseID        uuid.UUID
	ProjectID      uuid.UUID
	OrganizationID uuid.UUID
	ScopeID        uuid.UUID
	ProjectEpoch   int64
	Commands       []writefence.Command
	ManifestSHA256 string
}

// Validate binds every original command to the requested project and scope.
func (r ProjectWorkRequest) Validate() error {
	if r.CloseID == uuid.Nil || r.ProjectID == uuid.Nil || r.OrganizationID == uuid.Nil || r.ScopeID == uuid.Nil || r.ProjectEpoch < 1 || len(r.Commands) > 128 || !projectWorkHash(r.ManifestSHA256) {
		return writefence.ErrInvalid
	}
	seen := make(map[uuid.UUID]bool, len(r.Commands))
	for _, command := range r.Commands {
		if _, err := command.AuthorityDigest(); err != nil || seen[command.ID] {
			return writefence.ErrInvalid
		}
		seen[command.ID] = true
		project, scope := false, false
		for _, authority := range command.Authorities {
			if authority.Issuer != writefence.Creation {
				continue
			}
			switch authority.Kind {
			case writefence.Project:
				if authority.ID != r.ProjectID || authority.Epoch > r.ProjectEpoch {
					return writefence.ErrInvalid
				}
				project = true
			case writefence.Scope:
				if authority.ID != r.ScopeID {
					return writefence.ErrInvalid
				}
				scope = true
			}
		}
		if !project || !scope {
			return writefence.ErrInvalid
		}
	}
	return nil
}

func projectWorkHash(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == sha256.Size && hex.EncodeToString(raw) == value
}

// ProjectWorkState reports responsibility independently of scheduler presence.
type ProjectWorkState string

// Project responsibility remains unknown until all original evidence is conclusive.
const (
	ProjectWorkIdle    ProjectWorkState = "idle"
	ProjectWorkBusy    ProjectWorkState = "busy"
	ProjectWorkUnknown ProjectWorkState = "unknown"
)

// ProjectCommandObservation binds one local receipt to the exact original command.
type ProjectCommandObservation struct {
	Command       writefence.Command
	State         writefence.State
	ReceiptSHA256 string
}

// ProjectWorkSnapshot combines business responsibility and immutable command facts.
type ProjectWorkSnapshot struct {
	Request       ProjectWorkRequest
	BusinessState ProjectWorkState
	Commands      []ProjectCommandObservation
}

// ProjectWorkReader reads business and technical facts in a single local snapshot.
type ProjectWorkReader interface {
	ReadProjectWork(context.Context, ProjectWorkRequest) (ProjectWorkSnapshot, error)
}
