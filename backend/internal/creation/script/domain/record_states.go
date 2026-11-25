package domain

// SourceOrigin identifies the authority that first supplied a source snapshot.
type SourceOrigin string

// Source origins distinguish editor, provider and imported-file snapshots.
const (
	SourceOriginManual SourceOrigin = "manual"
	SourceOriginBeeftv SourceOrigin = "beeftv"
	SourceOriginFile   SourceOrigin = "file"
)

// SourceKind is the closed class of script inputs supported by the domain.
type SourceKind string

// Source kinds identify chapter, episode and original-document inputs.
const (
	SourceKindChapter  SourceKind = "chapter"
	SourceKindEpisode  SourceKind = "episode"
	SourceKindDocument SourceKind = "document"
)

// SourceStatus describes an editor-owned source snapshot.
type SourceStatus string

// Source statuses record progress of an editor-owned source.
const (
	SourceStatusDraft     SourceStatus = "draft"
	SourceStatusReady     SourceStatus = "ready"
	SourceStatusCompleted SourceStatus = "completed"
)

// SourceAction is the closed set of atomic source commands.
type SourceAction string

// Source actions are the accepted atomic mutations of source snapshots.
const (
	SourceActionCreate  SourceAction = "create"
	SourceActionUpdate  SourceAction = "update"
	SourceActionImport  SourceAction = "import"
	SourceActionDelete  SourceAction = "delete"
	SourceActionReorder SourceAction = "reorder"
)

// SourceControlAction selects explicit cancellation or reconciliation of a write.
type SourceControlAction string

// Source control actions explicitly cancel or reconcile a durable write.
const (
	SourceControlActionCancel    SourceControlAction = "cancel"
	SourceControlActionReconcile SourceControlAction = "reconcile"
)

// SplitSetKind distinguishes proposed boundaries from confirmed boundaries.
type SplitSetKind string

// Split set kinds distinguish proposed and accepted episode boundaries.
const (
	SplitSetKindCandidate SplitSetKind = "candidate"
	SplitSetKindFormal    SplitSetKind = "formal"
)

// SplitSetOrigin records the rule that produced a split set.
type SplitSetOrigin string

// Split set origins identify how episode boundaries were produced.
const (
	SplitSetOriginSources SplitSetOrigin = "sources"
	SplitSetOriginRules   SplitSetOrigin = "rules"
	SplitSetOriginManual  SplitSetOrigin = "manual"
)

// EpisodeInheritStatus records whether an episode retained identity across a split.
type EpisodeInheritStatus string

// Episode inheritance states preserve the result of identity matching across splits.
const (
	EpisodeInheritStatusNotInherited EpisodeInheritStatus = "not_inherited"
	EpisodeInheritStatusPending      EpisodeInheritStatus = "pending"
	EpisodeInheritStatusInherited    EpisodeInheritStatus = "inherited"
	EpisodeInheritStatusRetained     EpisodeInheritStatus = "retained"
	EpisodeInheritStatusInvalidated  EpisodeInheritStatus = "invalidated"
)

// StructureItemType distinguishes a dialogue item from an action item.
type StructureItemType string

// Structure item types distinguish spoken lines from scene actions.
const (
	StructureItemTypeDialogue StructureItemType = "line"
	StructureItemTypeAction   StructureItemType = "action"
)

// DialogueKind is the closed set of supported spoken-line forms.
type DialogueKind string

// Dialogue kinds distinguish dialogue, voiceover and inner speech.
const (
	DialogueKindDialogue  DialogueKind = "dialogue"
	DialogueKindVoiceover DialogueKind = "voiceover"
	DialogueKindInner     DialogueKind = "inner"
)

// StructureReviewStatus records the formal review transition of a structure.
type StructureReviewStatus string

// Structure review states distinguish candidates from confirmed structures.
const (
	StructureReviewStatusCandidate StructureReviewStatus = "candidate"
	StructureReviewStatusConfirmed StructureReviewStatus = "confirmed"
)
