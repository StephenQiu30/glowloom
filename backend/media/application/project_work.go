package application

import (
	"context"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
	"github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

// ProjectWorkRequest binds an observation to Creation's original closure and
// stable source-ledger manifest. Reading it never admits or freezes a command.
type ProjectWorkRequest struct {
	CloseID, ProjectID, OrganizationID, ScopeID uuid.UUID
	ProjectEpoch                                int64
	Commands                                    []writefence.Command
	ManifestSHA256                              string
}

// Validate bounds one manifest page and rejects foreign project authorities.
func (r ProjectWorkRequest) Validate() error {
	if r.CloseID == uuid.Nil || r.ProjectID == uuid.Nil || r.OrganizationID == uuid.Nil || r.ScopeID == uuid.Nil || r.ProjectEpoch < 1 || len(r.Commands) > 128 || !domain.ReferencePinDigestValid(r.ManifestSHA256) {
		return writefence.ErrInvalid
	}
	seen := make(map[uuid.UUID]bool, len(r.Commands))
	for _, command := range r.Commands {
		if _, err := command.AuthorityDigest(); err != nil || seen[command.ID] {
			return writefence.ErrInvalid
		}
		seen[command.ID] = true
		scope, project := false, false
		for _, authority := range command.Authorities {
			if authority.Issuer != writefence.Creation {
				continue
			}
			switch authority.Kind {
			case writefence.Scope:
				if authority.ID != r.ScopeID {
					return writefence.ErrInvalid
				}
				scope = true
			case writefence.Project:
				if authority.ID != r.ProjectID || authority.Epoch != r.ProjectEpoch {
					return writefence.ErrInvalid
				}
				project = true
			}
		}
		if !scope || !project {
			return writefence.ErrInvalid
		}
	}
	return nil
}

// ProjectBusinessState describes observed work, not permission to close a project.
type ProjectBusinessState string

// Project work observations distinguish a settled owner from live or uncertain work.
const (
	ProjectWorkIdle    ProjectBusinessState = "idle"
	ProjectWorkBusy    ProjectBusinessState = "busy"
	ProjectWorkUnknown ProjectBusinessState = "unknown"
)

// ProjectCommandObservation preserves one complete original command identity.
// Unknown cannot be interpreted as permanently absent or not adopted.
type ProjectCommandObservation struct {
	Command       writefence.Command
	State         string
	ReceiptSHA256 string
}

// ProjectWorkSnapshot is a consistent read of Media's retained responsibilities.
type ProjectWorkSnapshot struct {
	Request       ProjectWorkRequest
	BusinessState ProjectBusinessState
	Commands      []ProjectCommandObservation
}

// ProjectWorkReader is the read-only participant used by Creation's close gate.
type ProjectWorkReader interface {
	ReadProjectWork(context.Context, ProjectWorkRequest) (ProjectWorkSnapshot, error)
}
