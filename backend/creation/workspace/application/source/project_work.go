package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// ProjectWorkPageSize bounds each complete original-command request.
const ProjectWorkPageSize = 128

// ProjectWorkState describes the participant's real project responsibility, not
// the existence of a local scheduler row or a technical command receipt.
type ProjectWorkState string

// Participant business states remain independent of technical receipt states.
const (
	ProjectWorkIdle    ProjectWorkState = "idle"
	ProjectWorkBusy    ProjectWorkState = "busy"
	ProjectWorkUnknown ProjectWorkState = "unknown"
)

// ProjectWorkRequest contains only the source gate and its registered commands.
// It is a read request and carries no actor, credential, admission or write permit.
type ProjectWorkRequest struct {
	CloseID        uuid.UUID            `json:"close_id"`
	ProjectID      uuid.UUID            `json:"project_id"`
	OrganizationID uuid.UUID            `json:"organization_id"`
	ScopeID        uuid.UUID            `json:"scope_id"`
	ProjectEpoch   int64                `json:"project_epoch"`
	Commands       []writefence.Command `json:"commands"`
	ManifestSHA256 string               `json:"manifest_sha256"`
}

// Validate rejects incomplete resources and unbound project authorities.
func (r ProjectWorkRequest) Validate() error {
	if r.CloseID == uuid.Nil || r.ProjectID == uuid.Nil || r.OrganizationID == uuid.Nil || r.ScopeID == uuid.Nil || r.ProjectEpoch < 1 || len(r.Commands) > ProjectWorkPageSize || !validPreparationHash(r.ManifestSHA256) {
		return writefence.ErrInvalid
	}
	seen := make(map[uuid.UUID]bool, len(r.Commands))
	for _, command := range r.Commands {
		if _, err := command.AuthorityDigest(); err != nil || seen[command.ID] {
			return writefence.ErrInvalid
		}
		seen[command.ID] = true
		found := false
		for _, authority := range command.Authorities {
			if authority.Issuer == writefence.Creation && authority.Kind == writefence.Project {
				if authority.ID != r.ProjectID || authority.Epoch > r.ProjectEpoch {
					return writefence.ErrInvalid
				}
				found = true
			}
		}
		if !found {
			return writefence.ErrInvalid
		}
	}
	return nil
}

// Fingerprint binds complete original commands, including authority generations.
func (r ProjectWorkRequest) Fingerprint() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	encoded, err := writefence.CanonicalReceipt(raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(append([]byte("glowloom.creation.project_work\x00"), encoded...))
	return hex.EncodeToString(sum[:]), nil
}

// ProjectCommandObservation reports one exact technical responsibility.
type ProjectCommandObservation struct {
	Command       writefence.Command `json:"command"`
	State         writefence.State   `json:"state"`
	ReceiptSHA256 string             `json:"receipt_sha256,omitempty"`
}

// ProjectWorkSnapshot echoes the complete request. A committed command may still
// have busy/unknown business responsibility (for example an accepted input with
// no final operation/cancellation proof), so both dimensions must be inspected.
type ProjectWorkSnapshot struct {
	Request       ProjectWorkRequest          `json:"request"`
	BusinessState ProjectWorkState            `json:"business_state"`
	Commands      []ProjectCommandObservation `json:"commands"`
}

// ProjectWorkReader inspects current owner responsibility without granting writes.
type ProjectWorkReader interface {
	ReadProjectWork(context.Context, ProjectWorkRequest) (ProjectWorkSnapshot, error)
}

// Inspect rejects partial, retargeted, duplicated or malformed observations. It
// accepts a different observation order but never a missing original command.
func (s ProjectWorkSnapshot) Inspect(expected ProjectWorkRequest) (ProjectWorkState, error) {
	want, err := expected.Fingerprint()
	if err != nil {
		return "", err
	}
	got, err := s.Request.Fingerprint()
	if err != nil || want != got || len(s.Commands) != len(expected.Commands) {
		return "", writefence.ErrConflict
	}
	state := s.BusinessState
	if state != ProjectWorkIdle && state != ProjectWorkBusy && state != ProjectWorkUnknown {
		return "", writefence.ErrUnavailable
	}
	originals := make(map[uuid.UUID]writefence.Command, len(expected.Commands))
	for _, command := range expected.Commands {
		originals[command.ID] = command
	}
	seen := make(map[uuid.UUID]bool, len(s.Commands))
	for _, observation := range s.Commands {
		original, ok := originals[observation.Command.ID]
		if !ok || seen[original.ID] {
			return "", writefence.ErrConflict
		}
		seen[original.ID] = true
		binding, bindingErr := observation.Command.AuthorityDigest()
		originalBinding, _ := original.AuthorityDigest()
		if bindingErr != nil || binding != originalBinding || observation.Command.Action != original.Action || observation.Command.SHA256 != original.SHA256 {
			return "", writefence.ErrConflict
		}
		switch observation.State {
		case writefence.Committed:
			if !validPreparationHash(observation.ReceiptSHA256) {
				return "", writefence.ErrConflict
			}
		case writefence.Frozen:
			if observation.ReceiptSHA256 != "" && !validPreparationHash(observation.ReceiptSHA256) {
				return "", writefence.ErrConflict
			}
		case writefence.Admitted:
			if observation.ReceiptSHA256 != "" {
				return "", writefence.ErrConflict
			}
			state = ProjectWorkBusy
		case writefence.State("unknown"):
			if observation.ReceiptSHA256 != "" {
				return "", writefence.ErrConflict
			}
			if state != ProjectWorkBusy {
				state = ProjectWorkUnknown
			}
		default:
			return "", writefence.ErrUnavailable
		}
	}
	return state, nil
}

// ErrProjectClosureBusy preserves the original inflight-work rejection.
var ErrProjectClosureBusy = errors.New("project has inflight work")

// ErrProjectClosureUnknown retains an unresolved original close decision.
var ErrProjectClosureUnknown = errors.New("project closure responsibility unresolved")

// ProjectWorkFailure preserves the originating error while distinguishing an
// unresolved responsibility from known busy work at the public boundary.
func projectWorkFailure(target writefence.Writer, err error) error {
	return fmt.Errorf("project work participant %s: %w: %w", target, ErrProjectClosureUnknown, err)
}
