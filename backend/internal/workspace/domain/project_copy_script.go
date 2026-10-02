package domain

import (
	"math"

	"github.com/google/uuid"
)

// ProjectCopyScriptCounts records each owning historical row set and private object.
type ProjectCopyScriptCounts struct {
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

// ProjectCopyScriptSnapshot freezes own history evidence without content or object keys.
type ProjectCopyScriptSnapshot struct {
	ID             uuid.UUID               `json:"id"`
	ManifestSHA256 string                  `json:"manifest_sha256"`
	ContentSHA256  string                  `json:"content_sha256"`
	Counts         ProjectCopyScriptCounts `json:"counts"`
}

// ProjectCopyScriptReceipt proves every historical row and object after registration.
type ProjectCopyScriptReceipt struct {
	ManifestSHA256 string                  `json:"manifest_sha256"`
	ContentSHA256  string                  `json:"content_sha256"`
	Counts         ProjectCopyScriptCounts `json:"counts"`
}

// AcceptScriptReceipt advances only after the complete frozen history is verified.
func (j *ProjectCopyJob) AcceptScriptReceipt(worker uuid.UUID, receipt ProjectCopyScriptReceipt) error {
	if err := j.worker(worker); err != nil {
		return err
	}
	if j.Status != "running" || j.Stage != "script" || j.MediaReceipt == nil || j.CancellationRequested {
		return ErrProjectCopyStateConflict
	}
	if !validCopyScriptReceipt(receipt, j.Manifest.Script) {
		return ErrInvalidProjectCopy
	}
	if err := j.advance(); err != nil {
		return err
	}
	j.ScriptReceipt, j.Stage = &receipt, "canvases"
	return nil
}

func validCopyScriptCounts(c ProjectCopyScriptCounts) bool {
	for _, count := range [...]int{c.Sources, c.Versions, c.VersionSources, c.ProjectStates, c.VersionHeads, c.SplitSets, c.SplitConfirmations, c.Episodes, c.Structures, c.Scenes, c.DialogueLines, c.ActionLines, c.Objects} {
		if count < 0 || count > math.MaxInt32 {
			return false
		}
	}
	return c.ProjectStates <= 1 && c.VersionHeads == c.Versions
}

func validCopyScriptReceipt(r ProjectCopyScriptReceipt, s *ProjectCopyScriptSnapshot) bool {
	return s != nil && r.ManifestSHA256 == s.ManifestSHA256 && r.ContentSHA256 == s.ContentSHA256 && r.Counts == s.Counts
}

func (j ProjectCopyJob) validateScript() error {
	s := j.Manifest.Script
	if s != nil && (s.ID == uuid.Nil || !copyDigest(s.ManifestSHA256) || !copyDigest(s.ContentSHA256) || !validCopyScriptCounts(s.Counts)) {
		return ErrInvalidProjectCopy
	}
	if j.ScriptReceipt != nil && (!validCopyScriptReceipt(*j.ScriptReceipt, s) || j.MediaReceipt == nil) {
		return ErrInvalidProjectCopy
	}
	if j.Stage == "script" && (s == nil || j.MediaReceipt == nil || j.ScriptReceipt != nil || j.CanvasReceipt != nil) {
		return ErrInvalidProjectCopy
	}
	if j.Stage == "media" && j.ScriptReceipt != nil {
		return ErrInvalidProjectCopy
	}
	if s != nil && (j.Stage == "canvases" || j.Stage == "finalizing" || j.Status == "succeeded") && j.ScriptReceipt == nil {
		return ErrInvalidProjectCopy
	}
	return nil
}
