package domain

// OutputKindMedia identifies an operation result backed by a media asset.
const OutputKindMedia = "media"

// Origin identifies the trusted path that created a generation operation.
type Origin string

// Output origins identify the authority that created an operation result.
const (
	OriginPipeline Origin = "pipeline"
	OriginBatch    Origin = "batch"
	OriginCanvas   Origin = "canvas"
	OriginAgent    Origin = "agent"
)

// Capability and Mode identify the operation choices used for media processing filters.
const (
	CapabilityImageUpscale         = "image.upscale"
	CapabilityVideoUpscale         = "video.upscale"
	CapabilityImageSuperResolution = "image.super_resolution"
	CapabilityVideoSuperResolution = "video.super_resolution"
	ModeSuperResolution            = "super_resolution"
)
