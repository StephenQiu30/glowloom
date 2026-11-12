package domain

// Node types are persisted wire values shared by canvas projections and their
// business consumers. Keep them stable when adding new node behavior.
const (
	NodeTypeText            = "text"
	NodeTypeGroup           = "group"
	NodeTypeConfig          = "config"
	NodeTypeSubject         = "subject"
	NodeTypeImage           = "image"
	NodeTypeVideo           = "video"
	NodeTypeAudio           = "audio"
	NodeTypeModel           = "model"
	NodeTypeImageGeneration = "image_generation"
	NodeTypeVideoGeneration = "video_generation"
	NodeTypeCinemaStudio    = "cinema_studio"
	NodeTypeBatchTable      = "batch_table"
	NodeTypeTimeline        = "timeline"
	NodeTypeDirector        = "director"
	NodeTypeGeneration      = "generation"

	NodeActionResource = "resource"
	NodeActionTool     = "tool"
	NodeActionGenerate = "generate"
	RefTypeMediaAsset  = "media_asset"
)
