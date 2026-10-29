package editproject

import (
	"encoding/xml"
	"fmt"
	"math"
	"strconv"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

type fcpxml struct {
	XMLName   xml.Name     `xml:"fcpxml"`
	Version   string       `xml:"version,attr"`
	Resources fcpResources `xml:"resources"`
	Library   fcpLibrary   `xml:"library"`
}
type fcpResources struct {
	Formats []fcpFormat `xml:"format"`
	Assets  []fcpAsset  `xml:"asset"`
}
type fcpFormat struct {
	ID            string `xml:"id,attr"`
	FrameDuration string `xml:"frameDuration,attr"`
	Width         int32  `xml:"width,attr,omitempty"`
	Height        int32  `xml:"height,attr,omitempty"`
	ColorSpace    string `xml:"colorSpace,attr,omitempty"`
}
type fcpAsset struct {
	ID            string   `xml:"id,attr"`
	Name          string   `xml:"name,attr"`
	UID           string   `xml:"uid,attr"`
	Start         string   `xml:"start,attr"`
	Duration      string   `xml:"duration,attr,omitempty"`
	HasVideo      string   `xml:"hasVideo,attr,omitempty"`
	HasAudio      string   `xml:"hasAudio,attr,omitempty"`
	Format        string   `xml:"format,attr,omitempty"`
	AudioChannels int32    `xml:"audioChannels,attr,omitempty"`
	Media         fcpMedia `xml:"media-rep"`
}
type fcpMedia struct {
	Kind string `xml:"kind,attr"`
	Src  string `xml:"src,attr"`
}
type fcpLibrary struct {
	Event fcpEvent `xml:"event"`
}
type fcpEvent struct {
	Name    string     `xml:"name,attr"`
	Project fcpProject `xml:"project"`
}
type fcpProject struct {
	Name     string      `xml:"name,attr"`
	UID      string      `xml:"uid,attr"`
	Sequence fcpSequence `xml:"sequence"`
}
type fcpSequence struct {
	Format      string   `xml:"format,attr"`
	Duration    string   `xml:"duration,attr"`
	TCStart     string   `xml:"tcStart,attr"`
	TCFormat    string   `xml:"tcFormat,attr"`
	AudioLayout string   `xml:"audioLayout,attr"`
	AudioRate   string   `xml:"audioRate,attr"`
	Spine       fcpSpine `xml:"spine"`
}
type fcpSpine struct {
	Gap fcpGap `xml:"gap"`
}
type fcpGap struct {
	Name     string    `xml:"name,attr"`
	Start    string    `xml:"start,attr"`
	Offset   string    `xml:"offset,attr"`
	Duration string    `xml:"duration,attr"`
	Clips    []fcpClip `xml:"asset-clip"`
}
type fcpClip struct {
	Ref       string     `xml:"ref,attr"`
	Name      string     `xml:"name,attr"`
	Lane      int        `xml:"lane,attr"`
	Offset    string     `xml:"offset,attr"`
	Start     string     `xml:"start,attr"`
	Duration  string     `xml:"duration,attr"`
	Enabled   string     `xml:"enabled,attr"`
	SrcEnable string     `xml:"srcEnable,attr"`
	Volume    *fcpVolume `xml:"adjust-volume,omitempty"`
}
type fcpVolume struct {
	Amount string `xml:"amount,attr"`
}

func frames(ms, numerator, denominator int64) int64 {
	return int64(math.Round(float64(ms) * float64(numerator) / (1000 * float64(denominator))))
}
func rational(num, den int64) string {
	if num == 0 {
		return "0s"
	}
	a, b := num, den
	for b != 0 {
		a, b = b, a%b
	}
	return fmt.Sprintf("%d/%ds", num/a, den/a)
}
func frameTime(ms, numerator, denominator int64) string {
	return rational(frames(ms, numerator, denominator)*denominator, numerator)
}
func durationMS(frozen domain.FrozenExport) int64 {
	var result int64
	for _, clip := range frozen.Timeline.Clips {
		result = max(result, clip.StartMS+clip.DurationMS)
	}
	return result
}
func xmlBytes(value any, doctype string) ([]byte, error) {
	data, err := xml.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, err
	}
	return []byte(xml.Header + "<!DOCTYPE " + doctype + ">\n" + string(data) + "\n"), nil
}
func fcpDocument(frozen domain.FrozenExport) ([]byte, error) {
	width, height, err := frozen.VideoDimensions()
	if err != nil {
		return nil, err
	}
	name := "时间线"
	if frozen.Settings != nil {
		name = frozen.Settings.FileName
	}
	fps := int64(frozen.Timeline.FPS)
	doc := fcpxml{Version: "1.10", Resources: fcpResources{Formats: []fcpFormat{{ID: "sequence-format", FrameDuration: rational(1, fps), Width: int32(width), Height: int32(height), ColorSpace: "1-1-1 (Rec. 709)"}}}}
	refs := make(map[uuid.UUID]string)
	inputs := make(map[uuid.UUID]domain.FrozenSource)
	for i, input := range frozen.Inputs {
		ref := "asset-" + strconv.Itoa(i+1)
		refs[input.AssetID] = ref
		inputs[input.AssetID] = input
		asset := fcpAsset{ID: ref, Name: sourceName(input), UID: input.AssetID.String(), Start: "0s", Media: fcpMedia{Kind: "original-media", Src: "./Media/" + sourceName(input)}}
		if input.DurationMS != nil {
			asset.Duration = rational(int64(*input.DurationMS), 1000)
		}
		if input.Kind != "audio" {
			asset.HasVideo = "1"
			asset.Format = "source-format-" + strconv.Itoa(i+1)
			n, d := fps, int64(1)
			if input.FPS != nil {
				n, d, _ = frameRate(*input.FPS)
			}
			doc.Resources.Formats = append(doc.Resources.Formats, fcpFormat{ID: asset.Format, FrameDuration: rational(d, n), Width: *input.Width, Height: *input.Height})
		}
		if input.Kind == "audio" || input.AudioChannels != nil && *input.AudioChannels > 0 {
			asset.HasAudio = "1"
			if input.AudioChannels != nil {
				asset.AudioChannels = *input.AudioChannels
			}
		}
		doc.Resources.Assets = append(doc.Resources.Assets, asset)
	}
	gap := fcpGap{Name: name, Offset: "0s", Start: "0s", Duration: frameTime(durationMS(frozen), fps, 1)}
	for i, track := range frozen.Timeline.Tracks {
		for _, clip := range sortedClips(frozen.Timeline.Clips) {
			if clip.TrackID != track.ID {
				continue
			}
			input := inputs[*clip.AssetID]
			n, d := fps, int64(1)
			if input.FPS != nil {
				n, d, _ = frameRate(*input.FPS)
			}
			lane := i + 1
			if track.Kind == "audio" {
				lane = -lane
			}
			item := fcpClip{Ref: refs[*clip.AssetID], Name: clip.Title, Lane: lane, Offset: frameTime(clip.StartMS, fps, 1), Start: frameTime(clip.SourceStartMS, n, d), Duration: rational(frames(clip.StartMS+clip.DurationMS, fps, 1)-frames(clip.StartMS, fps, 1), fps), Enabled: "1", SrcEnable: "all"}
			if !track.Visible || clip.Kind == "audio" && (track.Muted || clip.Volume == 0) {
				item.Enabled = "0"
			}
			if clip.Kind == "audio" {
				item.SrcEnable = "audio"
			} else if track.Muted || clip.Volume == 0 {
				item.SrcEnable = "video"
			}
			if clip.Kind == "audio" || input.AudioChannels != nil && *input.AudioChannels > 0 {
				gain := -96.0
				if !track.Muted && clip.Volume > 0 {
					gain = 20 * math.Log10(clip.Volume)
				}
				item.Volume = &fcpVolume{Amount: strconv.FormatFloat(gain, 'f', 8, 64) + "dB"}
			}
			gap.Clips = append(gap.Clips, item)
		}
	}
	doc.Library.Event = fcpEvent{Name: "Glowloom", Project: fcpProject{Name: name, UID: frozen.Source.NodeID.String(), Sequence: fcpSequence{Format: "sequence-format", Duration: gap.Duration, TCStart: "0s", TCFormat: "NDF", AudioLayout: "stereo", AudioRate: "48k", Spine: fcpSpine{Gap: gap}}}}
	return xmlBytes(doc, "fcpxml")
}
