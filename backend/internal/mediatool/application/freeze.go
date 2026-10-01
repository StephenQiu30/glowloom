package application

import (
	"encoding/hex"
	"fmt"
	"slices"

	"github.com/google/uuid"

	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

// HasAudibleClip checks saved visibility, mute and gain; the worker must still
// verify that the immutable source bytes actually contain an audio stream.
func HasAudibleClip(config canvasdomain.TimelineConfig) bool {
	tracks := make(map[uuid.UUID]canvasdomain.TimelineTrack, len(config.Tracks))
	for _, track := range config.Tracks {
		tracks[track.ID] = track
	}
	for _, clip := range config.Clips {
		track := tracks[clip.TrackID]
		if track.Visible && !track.Muted && clip.Volume > 0 && (clip.Kind == "audio" || clip.Kind == "video") {
			return true
		}
	}
	return false
}

// SourceIDs collects resolved originals once; caller has already locked the canvas.
func SourceIDs(config canvasdomain.TimelineConfig) ([]uuid.UUID, error) {
	if config.Validate() != nil {
		return nil, ErrInvalidExport
	}
	var ids []uuid.UUID
	visible := make(map[uuid.UUID]bool, len(config.Tracks))
	for _, track := range config.Tracks {
		visible[track.ID] = track.Visible
	}
	hasClip := false
	for _, clip := range config.Clips {
		hasClip = hasClip || visible[clip.TrackID]
		if clip.Kind == "text" || clip.Kind == "subtitle" {
			continue
		}
		if clip.AssetID == nil {
			return nil, ErrInvalidExport
		}
		ids = append(ids, *clip.AssetID)
	}
	if !hasClip {
		return nil, ErrInvalidExport
	}
	slices.SortFunc(ids, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	return slices.Compact(ids), nil
}

// FreezeInputs rechecks source kind, actual media duration and exact crop bounds.
// SourceDurationMS is a client hint; authorization relies on stored probe facts.
func FreezeInputs(config canvasdomain.TimelineConfig, assets []mediadomain.MediaAsset) (domain.FrozenExport, error) {
	ids, err := SourceIDs(config)
	if err != nil {
		return domain.FrozenExport{}, err
	}
	byID := make(map[uuid.UUID]mediadomain.MediaAsset, len(assets))
	result := domain.FrozenExport{Timeline: config, Inputs: make([]domain.FrozenSource, 0, len(ids))}
	var total int64
	for _, asset := range assets {
		byID[asset.ID] = asset
	}
	for _, id := range ids {
		asset, ok := byID[id]
		if !ok || !asset.CanReference() || asset.SHA256 == nil || len(*asset.SHA256) != 64 || asset.ByteSize < 1 {
			return domain.FrozenExport{}, ErrInvalidExport
		}
		if _, err := hex.DecodeString(*asset.SHA256); err != nil {
			return domain.FrozenExport{}, ErrInvalidExport
		}
		total += asset.ByteSize
		if total > 2<<30 {
			return domain.FrozenExport{}, fmt.Errorf("%w: source byte budget", ErrInvalidExport)
		}
		result.Inputs = append(result.Inputs, domain.FrozenSource{AssetID: id, Revision: asset.Revision, Kind: string(asset.Kind), ObjectKey: asset.ObjectKey, MIMEType: asset.MimeType, ByteSize: asset.ByteSize, SHA256: *asset.SHA256, DurationMS: asset.DurationMS, Width: asset.Width, Height: asset.Height})
	}
	for _, clip := range config.Clips {
		if clip.AssetID == nil {
			continue
		}
		asset, ok := byID[*clip.AssetID]
		if !ok || string(asset.Kind) != clip.Kind {
			return domain.FrozenExport{}, ErrInvalidExport
		}
		if clip.Kind == "video" || clip.Kind == "audio" {
			if asset.DurationMS == nil || clip.SourceStartMS+clip.DurationMS > int64(*asset.DurationMS)+int64(1000/config.FPS) {
				return domain.FrozenExport{}, fmt.Errorf("%w: source duration", ErrInvalidExport)
			}
		}
		if crop := clip.Crop; crop != nil {
			if asset.Width == nil || asset.Height == nil || crop.X < 0 || crop.Y < 0 || crop.Width < 2 || crop.Height < 2 || int64(crop.X)+int64(crop.Width) > int64(*asset.Width) || int64(crop.Y)+int64(crop.Height) > int64(*asset.Height) || (clip.Kind != "image" && clip.Kind != "video") || (clip.Kind == "video" && (crop.Width%2 != 0 || crop.Height%2 != 0)) {
				return domain.FrozenExport{}, fmt.Errorf("%w: source crop", ErrInvalidExport)
			}
		}
	}
	return result, nil
}
