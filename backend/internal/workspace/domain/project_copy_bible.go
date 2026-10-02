package domain

import (
	"math"

	"github.com/google/uuid"
)

// ProjectCopyBibleCounts records every owned history set, including retired heads.
type ProjectCopyBibleCounts struct {
	Characters             int `json:"characters"`
	CharacterVersions      int `json:"character_versions"`
	CharacterConfirmations int `json:"character_confirmations"`
	Locations              int `json:"locations"`
	LocationVersions       int `json:"location_versions"`
	LocationConfirmations  int `json:"location_confirmations"`
	Props                  int `json:"props"`
	PropVersions           int `json:"prop_versions"`
	PropConfirmations      int `json:"prop_confirmations"`
	Looks                  int `json:"looks"`
	LookVersions           int `json:"look_versions"`
	References             int `json:"references"`
	Voices                 int `json:"voices"`
	Redirects              int `json:"redirects"`
	Splits                 int `json:"splits"`
}

// ProjectCopyBibleMapping declares the content owner's exact identity mapping.
type ProjectCopyBibleMapping struct {
	Kind     string    `json:"kind"`
	SourceID uuid.UUID `json:"source_id"`
	TargetID uuid.UUID `json:"target_id"`
}

// ProjectCopyBibleSnapshot contains safe evidence and declared foreign references.
type ProjectCopyBibleSnapshot struct {
	ID             uuid.UUID                 `json:"id"`
	ManifestSHA256 string                    `json:"manifest_sha256"`
	ContentSHA256  string                    `json:"content_sha256"`
	Counts         ProjectCopyBibleCounts    `json:"counts"`
	Identities     []ProjectCopyBibleMapping `json:"identities"`
	Versions       []ProjectCopyBibleMapping `json:"versions"`
}

// ProjectCopyBibleReceipt proves the complete frozen history after registration.
type ProjectCopyBibleReceipt struct {
	ManifestSHA256 string                 `json:"manifest_sha256"`
	ContentSHA256  string                 `json:"content_sha256"`
	Counts         ProjectCopyBibleCounts `json:"counts"`
}

func validCopyBibleCounts(c ProjectCopyBibleCounts) bool {
	for _, n := range [...]int{c.Characters, c.CharacterVersions, c.CharacterConfirmations, c.Locations, c.LocationVersions, c.LocationConfirmations, c.Props, c.PropVersions, c.PropConfirmations, c.Looks, c.LookVersions, c.References, c.Voices, c.Redirects, c.Splits} {
		if n < 0 || n > math.MaxInt32 {
			return false
		}
	}
	return true
}

func validCopyBibleMappings(values []ProjectCopyBibleMapping, characters, locations, props int) bool {
	counts := map[string]int{"character": 0, "location": 0, "prop": 0}
	type identity struct {
		kind string
		id   uuid.UUID
	}
	sources, targets := make(map[identity]bool, len(values)), make(map[identity]bool, len(values))
	for _, v := range values {
		source, target := identity{v.Kind, v.SourceID}, identity{v.Kind, v.TargetID}
		if _, valid := counts[v.Kind]; !valid || v.SourceID == uuid.Nil || v.TargetID == uuid.Nil || v.SourceID == v.TargetID || sources[source] || targets[target] {
			return false
		}
		sources[source], targets[target] = true, true
		counts[v.Kind]++
	}
	return counts["character"] == characters && counts["location"] == locations && counts["prop"] == props
}

func validCopyBibleReceipt(r ProjectCopyBibleReceipt, s *ProjectCopyBibleSnapshot) bool {
	return s != nil && r.ManifestSHA256 == s.ManifestSHA256 && r.ContentSHA256 == s.ContentSHA256 && r.Counts == s.Counts
}

// AcceptBibleReceipt advances only after complete history and physical media proof.
func (j *ProjectCopyJob) AcceptBibleReceipt(worker uuid.UUID, receipt ProjectCopyBibleReceipt) error {
	if err := j.worker(worker); err != nil {
		return err
	}
	if j.Status != "running" || j.Stage != "bible" || j.MediaReceipt == nil || j.CancellationRequested {
		return ErrProjectCopyStateConflict
	}
	if !validCopyBibleReceipt(receipt, j.Manifest.Bible) {
		return ErrInvalidProjectCopy
	}
	if err := j.advance(); err != nil {
		return err
	}
	j.BibleReceipt, j.Stage = &receipt, "canvases"
	if j.Manifest.Script != nil {
		j.Stage = "script"
	}
	return nil
}

func (j ProjectCopyJob) validateBible() error {
	s := j.Manifest.Bible
	if s != nil {
		c := s.Counts
		if s.ID == uuid.Nil || !copyDigest(s.ManifestSHA256) || !copyDigest(s.ContentSHA256) || !validCopyBibleCounts(c) || !validCopyBibleMappings(s.Identities, c.Characters, c.Locations, c.Props) || !validCopyBibleMappings(s.Versions, c.CharacterVersions, c.LocationVersions, c.PropVersions) {
			return ErrInvalidProjectCopy
		}
	}
	if j.BibleReceipt != nil && (!validCopyBibleReceipt(*j.BibleReceipt, s) || j.MediaReceipt == nil) {
		return ErrInvalidProjectCopy
	}
	if j.Stage == "bible" && (s == nil || j.MediaReceipt == nil || j.BibleReceipt != nil || j.ScriptReceipt != nil || j.CanvasReceipt != nil) {
		return ErrInvalidProjectCopy
	}
	if j.Stage == "media" && j.BibleReceipt != nil {
		return ErrInvalidProjectCopy
	}
	if s != nil && (j.Stage == "script" || j.Stage == "canvases" || j.Stage == "finalizing" || j.Status == "succeeded") && j.BibleReceipt == nil {
		return ErrInvalidProjectCopy
	}
	return nil
}
