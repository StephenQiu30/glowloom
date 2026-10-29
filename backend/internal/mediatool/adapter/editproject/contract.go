// Package editproject builds portable, identity-bound timeline interchange archives.
package editproject

import (
	"path"
	"slices"
	"strings"

	"github.com/google/uuid"

	canvas "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

// MaxBytes bounds the total bytes of a portable project archive.
const MaxBytes int64 = 500 << 20
const maxDocumentBytes = 4 << 20

// MediaEntry records exact packaged input bytes without exposing private object keys.
type MediaEntry struct {
	AssetID  uuid.UUID `json:"asset_id"`
	Revision int64     `json:"revision"`
	Path     string    `json:"path"`
	Kind     string    `json:"kind"`
	MIMEType string    `json:"mime_type"`
	ByteSize int64     `json:"byte_size"`
	SHA256   string    `json:"sha256"`
}

// Manifest preserves the exact editable source and names the supported output contract.
type Manifest struct {
	Version       int                   `json:"version"`
	Format        domain.OutputKind     `json:"format"`
	FormatVersion string                `json:"format_version"`
	Source        domain.Source         `json:"source"`
	Timeline      canvas.TimelineConfig `json:"timeline"`
	Media         []MediaEntry          `json:"media"`
	Notes         []string              `json:"notes"`
}

func extension(mime string) (string, bool) { return application.ProjectMediaExtension(mime) }

// Validate exposes the immutable artifact admission contract to adapter callers.
func Validate(frozen domain.FrozenExport) error  { return application.ValidateProjectExport(frozen) }
func frameRate(fps float64) (int64, int64, bool) { return application.ProjectSourceFrameRate(fps) }

func sourceName(source domain.FrozenSource) string {
	ext, _ := extension(source.MIMEType)
	return source.AssetID.String() + ext
}
func mediaPath(kind domain.OutputKind, name string) string {
	if kind == domain.OutputFCPXML {
		return path.Join("timeline.fcpxmld", "Media", name)
	}
	return path.Join("Media", name)
}
func manifest(frozen domain.FrozenExport) Manifest {
	m := Manifest{Version: 1, Format: frozen.OutputKind, FormatVersion: frozen.OutputKind.FormatVersion(), Source: *frozen.Source, Timeline: frozen.Timeline, Media: make([]MediaEntry, 0, len(frozen.Inputs)), Notes: []string{}}
	for _, input := range frozen.Inputs {
		m.Media = append(m.Media, MediaEntry{AssetID: input.AssetID, Revision: input.Revision, Path: mediaPath(frozen.OutputKind, sourceName(input)), Kind: input.Kind, MIMEType: input.MIMEType, ByteSize: input.ByteSize, SHA256: input.SHA256})
	}
	if frozen.OutputKind != domain.OutputMediaBundle && frozen.OutputKind != domain.OutputJianying {
		m.Notes = append(m.Notes, "Edit endpoints are rounded to the nearest sequence/source frame; the exact millisecond timeline remains in this manifest.", "Interchange validation is not target-application import acceptance.")
	}
	if frozen.OutputKind == domain.OutputXMEML {
		m.Notes = append(m.Notes, "Relink source media from the packaged Media directory if the importing application requires absolute file URLs.")
	}
	return m
}
func sortedClips(clips []canvas.TimelineClip) []canvas.TimelineClip {
	result := slices.Clone(clips)
	slices.SortFunc(result, func(a, b canvas.TimelineClip) int {
		if a.StartMS < b.StartMS {
			return -1
		}
		if a.StartMS > b.StartMS {
			return 1
		}
		return strings.Compare(a.ID.String(), b.ID.String())
	})
	return result
}
