package application

import (
	"context"
	"time"

	"github.com/google/uuid"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

// ProjectCopyBinding is trusted workspace evidence; no public handler accepts it.
type ProjectCopyBinding struct{ JobID, OrgID, SourceProjectID, TargetProjectID uuid.UUID }

// ScriptCopyCounts enumerates every actual own historical row and private object.
type ScriptCopyCounts struct {
	Sources            int `json:"sources"`
	Versions           int `json:"versions"`
	VersionSources     int `json:"version_sources"`
	ProjectStates      int `json:"project_states"`
	VersionHeads       int `json:"version_heads"`
	SplitSets          int `json:"split_sets"`
	SplitConfirmations int `json:"split_confirmations"`
	Episodes           int `json:"episodes"`
	Structures         int `json:"structures"`
	Scenes             int `json:"scenes"`
	DialogueLines      int `json:"dialogue_lines"`
	ActionLines        int `json:"action_lines"`
	Objects            int `json:"objects"`
}

// ProjectCopySnapshot exposes integrity/count evidence, not content or private keys.
type ProjectCopySnapshot struct {
	ID                            uuid.UUID
	ManifestSHA256, ContentSHA256 string
	Counts                        ScriptCopyCounts
}

// ProjectCopyReceipt proves full target registration, preserving source semantic hashes.
type ProjectCopyReceipt struct {
	ManifestSHA256, ContentSHA256 string
	Counts                        ScriptCopyCounts
}

// ProjectCopyAccess verifies the workspace-minted admission or actual worker fence.
// Its factory binds the current own SQL transaction and the exact trusted job mode.
type ProjectCopyAccess interface {
	Authorize(context.Context, identityapp.Principal, ProjectCopyBinding, bool) error
}

// ProjectCopyScene retains the actual row UUID and all scene metadata/order.
type ProjectCopyScene struct {
	ID, OrgID, ProjectID, StructureID, SceneKey uuid.UUID
	SeqNo                                       int
	Heading, LocationText, TimeOfDay            string
	Start, End                                  int
}

// ProjectCopyDialogue retains row identity separately from the stable editable key.
type ProjectCopyDialogue struct {
	ID, OrgID, ProjectID, SceneID, LineKey       uuid.UUID
	SeqNo                                        int
	Kind, Speaker, Content, Emotion, ContentHash string
	CharacterID                                  *uuid.UUID
	Start, End                                   int
}

// ProjectCopyAction preserves actual action row identity and item order.
type ProjectCopyAction struct {
	ID, OrgID, ProjectID, SceneID, LineKey uuid.UUID
	SeqNo                                  int
	Content                                string
	Start, End                             int
}

// ProjectCopyVersionSource preserves each actual ordered version membership.
type ProjectCopyVersionSource struct {
	OrgID, ProjectID, VersionID, SourceID uuid.UUID
	Position                              int
}

// ProjectCopyHistory includes every snapshot and head; execution/finance are excluded.
type ProjectCopyHistory struct {
	State              *domain.ProjectState       `json:"state,omitempty"`
	Sources            []domain.SourceRecord      `json:"sources"`
	Versions           []domain.ScriptVersion     `json:"versions"`
	VersionSources     []ProjectCopyVersionSource `json:"version_sources"`
	VersionHeads       []VersionHead              `json:"version_heads"`
	SplitSets          []domain.SplitSet          `json:"split_sets"`
	SplitConfirmations []domain.SplitConfirmation `json:"split_confirmations"`
	Episodes           []domain.Episode           `json:"episodes"`
	Structures         []domain.EpisodeStructure  `json:"structures"`
	Scenes             []ProjectCopyScene         `json:"scenes"`
	Dialogue           []ProjectCopyDialogue      `json:"dialogue_lines"`
	Actions            []ProjectCopyAction        `json:"action_lines"`
}

// ProjectCopyObject freezes one exact source/digest and one unpublished target key.
type ProjectCopyObject struct {
	Source     domain.ObjectFact `json:"source"`
	Target     domain.ObjectFact `json:"target"`
	Copied     bool              `json:"copied"`
	PutStarted bool              `json:"put_started"`
	Removed    bool              `json:"removed"`
}

// ProjectCopyManifest contains private typed history and its complete immutable transfers.
type ProjectCopyManifest struct {
	Binding       ProjectCopyBinding  `json:"binding"`
	Source        ProjectCopyHistory  `json:"source"`
	Target        ProjectCopyHistory  `json:"target"`
	Objects       []ProjectCopyObject `json:"objects"`
	Counts        ScriptCopyCounts    `json:"counts"`
	ContentSHA256 string              `json:"content_sha256"`
}

// ProjectCopyStore owns every copy intent and rechecks the current worker between I/O.
type ProjectCopyStore interface {
	Freeze(context.Context, identityapp.Principal, ProjectCopyBinding, map[uuid.UUID]uuid.UUID, time.Time) (ProjectCopySnapshot, error)
	Manifest(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot) (ProjectCopyManifest, error)
	BeginObject(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot, domain.ObjectFact) error
	CompleteObject(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot, domain.ObjectFact) error
	Register(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot) (ProjectCopyReceipt, error)
	BeginCleanup(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot) ([]ProjectCopyObject, error)
	BeginCleanupObject(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot, domain.ObjectFact) error
	CompleteCleanupObject(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot, domain.ObjectFact) error
	FinishCleanup(context.Context, identityapp.Principal, ProjectCopyBinding, ProjectCopySnapshot) error
}

// ProjectCopy coordinates SQL-owned immutable history and actual private object transfers.
type ProjectCopy struct {
	store   ProjectCopyStore
	objects PrivateObjects
}

// NewProjectCopy injects full own persistence and the existing private object boundary.
func NewProjectCopy(store ProjectCopyStore, objects PrivateObjects) *ProjectCopy {
	return &ProjectCopy{store: store, objects: objects}
}
