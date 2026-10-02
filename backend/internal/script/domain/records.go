package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ObjectFact freezes private bytes. It is never a public object URL or path input.
type ObjectFact struct {
	Key      string `json:"key"`
	SHA256   string `json:"sha256"`
	ByteSize int64  `json:"byte_size"`
	MIME     string `json:"mime"`
}

// Validate checks immutable integrity facts before any object operation.
func (o ObjectFact) Validate() error {
	if o.Key == "" || len(o.Key) > 512 || strings.Contains(o.Key, "..") || strings.ContainsAny(o.Key, "\\\x00") || strings.HasPrefix(o.Key, "/") || len(o.SHA256) != 64 || o.ByteSize < 0 || o.ByteSize > MaxHTTPBytes || o.MIME == "" {
		return ErrInvalidSource
	}
	for _, c := range o.SHA256 {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return ErrInvalidSource
		}
	}
	return nil
}

// SourceOriginMapping records only proven decoded or Word paragraph coordinates.
type SourceOriginMapping struct {
	Part         string `json:"part"`
	Paragraph    *int   `json:"paragraph,omitempty"`
	DecodedStart *int   `json:"decoded_start,omitempty"`
	DecodedEnd   *int   `json:"decoded_end,omitempty"`
	Start        int    `json:"span_start"`
	End          int    `json:"span_end"`
}

// SourceProvenance retains source-system labels without using them as own identifiers.
type SourceProvenance struct {
	ExternalID       *uuid.UUID                `json:"external_id,omitempty"`
	ExternalPosition *int                      `json:"external_position,omitempty"`
	Encoding         string                    `json:"encoding,omitempty"`
	Mapping          []SourceOriginMapping     `json:"mapping,omitempty"`
	Warnings         []SourceExtractionWarning `json:"warnings,omitempty"`
}

// SourceExtractionWarning is a closed formal fact about original content loss.
// It survives immutable source history and project copy independently of jobs.
type SourceExtractionWarning struct {
	Code      string `json:"code"`
	Count     int    `json:"count"`
	Paragraph *int   `json:"paragraph,omitempty"`
}

// ValidateSourceWarnings bounds actual parser facts without accepting arbitrary labels.
func ValidateSourceWarnings(warnings []SourceExtractionWarning) error {
	if len(warnings) > 64 {
		return ErrInvalidSource
	}
	seen := make(map[string]bool, len(warnings))
	for _, warning := range warnings {
		valid := false
		switch warning.Code {
		case "unsupported_document_element", "table_layout_flattened", "document_layout_omitted", "unsupported_heading_style", "paragraph_style_omitted", "unsupported_alignment", "paragraph_layout_omitted", "unsupported_paragraph_content", "unsupported_run_content", "embedded_media_omitted", "unsupported_hyperlink", "document_revision_omitted", "underline_style_omitted", "unsupported_text_color", "unsupported_highlight_color", "run_format_omitted", "duplicate_run_format", "unsupported_numbering_format", "numbering_missing_parent", "numbered_heading_as_paragraph":
			valid = true
		}
		identity := warning.Code
		if !valid || seen[identity] || warning.Count < 1 || warning.Count > MaxDocumentNodes || warning.Paragraph != nil && (*warning.Paragraph < 0 || *warning.Paragraph >= MaxDocumentNodes) {
			return ErrInvalidSource
		}
		seen[identity] = true
	}
	return nil
}

// SourceRecord is an immutable input snapshot; editable identity is LineageID.
type SourceRecord struct {
	ID                uuid.UUID        `json:"id"`
	OrgID             uuid.UUID        `json:"org_id"`
	ProjectID         uuid.UUID        `json:"project_id"`
	LineageID         uuid.UUID        `json:"source_lineage_id"`
	PreviousID        *uuid.UUID       `json:"previous_source_id,omitempty"`
	Revision          int64            `json:"source_revision"`
	Origin            string           `json:"origin"`
	Kind              string           `json:"source_kind"`
	Title             string           `json:"title"`
	Status            string           `json:"status"`
	RightsActorID     uuid.UUID        `json:"rights_actor_id"`
	RightsConfirmedAt time.Time        `json:"rights_confirmed_at"`
	MediaAssetID      *uuid.UUID       `json:"media_asset_id,omitempty"`
	MediaRevision     *int64           `json:"media_revision,omitempty"`
	MediaSHA256       *string          `json:"media_sha256,omitempty"`
	Original          ObjectFact       `json:"original"`
	Rich              ObjectFact       `json:"rich"`
	ContentHash       string           `json:"content_hash"`
	CharCount         int              `json:"char_count"`
	Provenance        SourceProvenance `json:"provenance"`
	CreatedAt         time.Time        `json:"created_at"`
}

// ScriptVersion freezes the complete ordered source manifest and canonical objects.
type ScriptVersion struct {
	ID                   uuid.UUID    `json:"id"`
	OrgID                uuid.UUID    `json:"org_id"`
	ProjectID            uuid.UUID    `json:"project_id"`
	VersionNo            int64        `json:"version_no"`
	SourceIDs            []uuid.UUID  `json:"source_ids"`
	Spans                []SourceSpan `json:"source_spans"`
	Text                 ObjectFact   `json:"text"`
	Rich                 ObjectFact   `json:"rich"`
	ContentHash          string       `json:"content_hash"`
	DocumentSHA256       string       `json:"document_sha256"`
	SourceManifestSHA256 string       `json:"source_manifest_sha256"`
	CharCount            int          `json:"char_count"`
	CreatedAt            time.Time    `json:"created_at"`
}

// ProjectState is the small CAS head; a missing row reads as revision zero.
type ProjectState struct {
	ProjectID        uuid.UUID  `json:"project_id"`
	OrgID            uuid.UUID  `json:"org_id"`
	Revision         int64      `json:"revision"`
	DraftVersionID   *uuid.UUID `json:"draft_version_id,omitempty"`
	AdoptedVersionID *uuid.UUID `json:"adopted_version_id,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// SplitSet is an immutable candidate or formal group, retaining its actual rule origin.
type SplitSet struct {
	ID         uuid.UUID         `json:"id"`
	VersionID  uuid.UUID         `json:"version_id"`
	OrgID      uuid.UUID         `json:"org_id"`
	ProjectID  uuid.UUID         `json:"project_id"`
	Kind       string            `json:"kind"`
	Origin     string            `json:"origin"`
	Preface    *ScalarSpan       `json:"preface,omitempty"`
	Boundaries []EpisodeBoundary `json:"boundaries"`
	Warnings   []string          `json:"warnings,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

// Episode is the formal mutable head over immutable source and structure history.
type Episode struct {
	ID                   uuid.UUID  `json:"id"`
	OrgID                uuid.UUID  `json:"org_id"`
	ProjectID            uuid.UUID  `json:"project_id"`
	VersionID            uuid.UUID  `json:"script_version_id"`
	SplitSetID           uuid.UUID  `json:"split_set_id"`
	SeqNo                int        `json:"seq_no"`
	Title                string     `json:"title"`
	Start                int        `json:"span_start"`
	End                  int        `json:"span_end"`
	Revision             int64      `json:"revision"`
	CurrentStructureID   *uuid.UUID `json:"current_structure_id,omitempty"`
	ConfirmedStructureID *uuid.UUID `json:"confirmed_structure_id,omitempty"`
	PreviousEpisodeID    *uuid.UUID `json:"previous_episode_id,omitempty"`
	InheritStatus        string     `json:"inherit_status"`
	IsDelete             bool       `json:"is_delete"`
}

// SplitConfirmation preserves each complete boundary/identity result independently.
type SplitConfirmation struct {
	ID             uuid.UUID   `json:"id"`
	OrgID          uuid.UUID   `json:"org_id"`
	ProjectID      uuid.UUID   `json:"project_id"`
	VersionID      uuid.UUID   `json:"version_id"`
	CandidateSetID uuid.UUID   `json:"candidate_set_id"`
	FormalSetID    uuid.UUID   `json:"formal_set_id"`
	ActorID        uuid.UUID   `json:"actor_id"`
	Revision       int64       `json:"revision"`
	Preface        *ScalarSpan `json:"preface,omitempty"`
	Episodes       []Episode   `json:"episodes"`
	CreatedAt      time.Time   `json:"created_at"`
}

// ErrInvalidStructure rejects unsupported or inconsistent manual structure facts.
var ErrInvalidStructure = errors.New("invalid episode structure")

// StructureItem preserves the source scene's ordered action/dialogue interleaving.
type StructureItem struct {
	Type               string     `json:"type"`
	Key                uuid.UUID  `json:"line_key"`
	Kind               string     `json:"kind,omitempty"`
	Speaker            string     `json:"speaker_text,omitempty"`
	CharacterID        *uuid.UUID `json:"character_id,omitempty"`
	CharacterVersionID *uuid.UUID `json:"character_version_id,omitempty"`
	Content            string     `json:"content"`
	Emotion            string     `json:"emotion,omitempty"`
	Start              int        `json:"span_start"`
	End                int        `json:"span_end"`
}

// StructureScene retains formal scene metadata and the exact item order.
type StructureScene struct {
	Key          uuid.UUID       `json:"scene_key"`
	SeqNo        int             `json:"seq_no"`
	Heading      string          `json:"heading"`
	LocationText string          `json:"location_text"`
	TimeOfDay    string          `json:"time_of_day"`
	Start        int             `json:"span_start"`
	End          int             `json:"span_end"`
	Items        []StructureItem `json:"items"`
}

// UnassignedLine explicitly retains source text that still requires review.
type UnassignedLine struct {
	Key     uuid.UUID `json:"line_key"`
	Content string    `json:"content"`
	Start   int       `json:"span_start"`
	End     int       `json:"span_end"`
}

// StructureDocument is the complete closed manual structure input.
type StructureDocument struct {
	Scenes     []StructureScene `json:"scenes"`
	Unassigned []UnassignedLine `json:"unassigned_lines"`
}

// EpisodeStructure is immutable; current/confirmed pointers live only on Episode.
type EpisodeStructure struct {
	ID         uuid.UUID         `json:"id"`
	OrgID      uuid.UUID         `json:"org_id"`
	ProjectID  uuid.UUID         `json:"project_id"`
	EpisodeID  uuid.UUID         `json:"episode_id"`
	VersionNo  int64             `json:"version_no"`
	SourceHash string            `json:"source_hash"`
	Document   StructureDocument `json:"document"`
	ActorID    uuid.UUID         `json:"actor_id"`
	CreatedAt  time.Time         `json:"created_at"`
}
