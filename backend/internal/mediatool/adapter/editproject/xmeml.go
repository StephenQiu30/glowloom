package editproject

import (
	"encoding/xml"
	"strconv"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

type xmeml struct {
	XMLName  xml.Name  `xml:"xmeml"`
	Version  string    `xml:"version,attr"`
	Sequence xSequence `xml:"sequence"`
}
type xRate struct {
	Timebase int64  `xml:"timebase"`
	NTSC     string `xml:"ntsc"`
}
type xTimecode struct {
	Rate          xRate  `xml:"rate"`
	String        string `xml:"string"`
	Frame         int64  `xml:"frame"`
	DisplayFormat string `xml:"displayformat"`
}
type xSequence struct {
	ID       string    `xml:"id,attr"`
	Name     string    `xml:"name"`
	Duration int64     `xml:"duration"`
	Rate     xRate     `xml:"rate"`
	Timecode xTimecode `xml:"timecode"`
	Media    xMedia    `xml:"media"`
}
type xMedia struct {
	Video xVideo `xml:"video"`
	Audio xAudio `xml:"audio"`
}
type xVideo struct {
	Format xFormat  `xml:"format"`
	Tracks []xTrack `xml:"track"`
}
type xAudio struct {
	Tracks []xTrack `xml:"track"`
}
type xFormat struct {
	Characteristics xCharacteristics `xml:"samplecharacteristics"`
}
type xCharacteristics struct {
	Rate           *xRate `xml:"rate,omitempty"`
	Width          int32  `xml:"width,omitempty"`
	Height         int32  `xml:"height,omitempty"`
	Anamorphic     string `xml:"anamorphic,omitempty"`
	PixelAspect    string `xml:"pixelaspectratio,omitempty"`
	FieldDominance string `xml:"fielddominance,omitempty"`
	Depth          int    `xml:"depth,omitempty"`
	SampleRate     int    `xml:"samplerate,omitempty"`
}
type xTrack struct {
	Clips   []xClip `xml:"clipitem"`
	Enabled string  `xml:"enabled"`
	Locked  string  `xml:"locked"`
}
type xClip struct {
	ID       string   `xml:"id,attr"`
	Name     string   `xml:"name"`
	Enabled  string   `xml:"enabled"`
	Duration int64    `xml:"duration"`
	Rate     xRate    `xml:"rate"`
	Start    int64    `xml:"start"`
	End      int64    `xml:"end"`
	In       int64    `xml:"in"`
	Out      int64    `xml:"out"`
	File     xFile    `xml:"file"`
	Source   xSource  `xml:"sourcetrack"`
	Filter   *xFilter `xml:"filter,omitempty"`
	Links    []xLink  `xml:"link"`
}
type xLink struct {
	Ref   string `xml:"linkclipref"`
	Group int    `xml:"groupindex,omitempty"`
}
type xSource struct {
	MediaType  string `xml:"mediatype"`
	TrackIndex int    `xml:"trackindex"`
}
type xFile struct {
	ID       string     `xml:"id,attr"`
	Name     string     `xml:"name"`
	Path     string     `xml:"pathurl"`
	Rate     xRate      `xml:"rate"`
	Duration int64      `xml:"duration"`
	Media    xFileMedia `xml:"media"`
}
type xFileMedia struct {
	Video *xFileVideo `xml:"video,omitempty"`
	Audio *xFileAudio `xml:"audio,omitempty"`
}
type xFileVideo struct {
	Characteristics xCharacteristics `xml:"samplecharacteristics"`
}
type xFileAudio struct {
	Characteristics *xCharacteristics `xml:"samplecharacteristics,omitempty"`
	Channels        int32             `xml:"channelcount,omitempty"`
}
type xFilter struct {
	Effect xEffect `xml:"effect"`
}
type xEffect struct {
	Name      string     `xml:"name"`
	ID        string     `xml:"effectid"`
	Category  string     `xml:"effectcategory"`
	Type      string     `xml:"effecttype"`
	MediaType string     `xml:"mediatype"`
	Parameter xParameter `xml:"parameter"`
}
type xParameter struct {
	ID    string  `xml:"parameterid"`
	Name  string  `xml:"name"`
	Min   int     `xml:"valuemin"`
	Max   int     `xml:"valuemax"`
	Value float64 `xml:"value"`
}

func xRateFor(n, d int64) xRate {
	if d == 1001 {
		return xRate{Timebase: n / 1000, NTSC: "TRUE"}
	}
	return xRate{Timebase: n, NTSC: "FALSE"}
}
func boolText(value bool) string {
	if value {
		return "TRUE"
	}
	return "FALSE"
}
func xmemlDocument(frozen domain.FrozenExport) ([]byte, error) {
	width, height, err := frozen.VideoDimensions()
	if err != nil {
		return nil, err
	}
	name := "时间线"
	if frozen.Settings != nil {
		name = frozen.Settings.FileName
	}
	fps := int64(frozen.Timeline.FPS)
	rate := xRateFor(fps, 1)
	seq := xSequence{ID: "sequence-" + frozen.Source.NodeID.String(), Name: name, Duration: frames(durationMS(frozen), fps, 1), Rate: rate, Timecode: xTimecode{Rate: rate, String: "00:00:00:00", DisplayFormat: "NDF"}, Media: xMedia{Video: xVideo{Format: xFormat{Characteristics: xCharacteristics{Rate: &rate, Width: int32(width), Height: int32(height), Anamorphic: "FALSE", PixelAspect: "square", FieldDominance: "none"}}}}}
	inputs := make(map[uuid.UUID]domain.FrozenSource)
	for _, input := range frozen.Inputs {
		inputs[input.AssetID] = input
	}
	for _, track := range frozen.Timeline.Tracks {
		video := xTrack{Enabled: boolText(track.Visible), Locked: boolText(track.Locked)}
		audio := []xTrack{}
		for _, clip := range sortedClips(frozen.Timeline.Clips) {
			if clip.TrackID != track.ID {
				continue
			}
			input := inputs[*clip.AssetID]
			n, d := fps, int64(1)
			if input.FPS != nil {
				n, d, _ = frameRate(*input.FPS)
			}
			sourceRate := xRateFor(n, d)
			sourceDuration := clip.SourceStartMS + clip.DurationMS
			if input.DurationMS != nil {
				sourceDuration = int64(*input.DurationMS)
			}
			file := xFile{ID: "file-" + input.AssetID.String(), Name: sourceName(input), Path: "./Media/" + sourceName(input), Rate: sourceRate, Duration: frames(sourceDuration, n, d)}
			if input.Kind != "audio" {
				file.Media.Video = &xFileVideo{Characteristics: xCharacteristics{Rate: &sourceRate, Width: *input.Width, Height: *input.Height, Anamorphic: "FALSE", PixelAspect: "square", FieldDominance: "none"}}
			}
			hasAudio := input.Kind == "audio" || input.AudioChannels != nil && *input.AudioChannels > 0
			if hasAudio {
				channels := int32(0)
				if input.AudioChannels != nil {
					channels = *input.AudioChannels
				}
				file.Media.Audio = &xFileAudio{Channels: channels, Characteristics: nil}
			}
			item := xClip{ID: "clip-" + clip.ID.String(), Name: clip.Title, Enabled: boolText(track.Visible), Rate: sourceRate, Duration: frames(sourceDuration, n, d), Start: frames(clip.StartMS, fps, 1), End: frames(clip.StartMS+clip.DurationMS, fps, 1), In: frames(clip.SourceStartMS, n, d), Out: frames(clip.SourceStartMS+clip.DurationMS, n, d), File: file, Source: xSource{MediaType: "video", TrackIndex: 1}}
			channels := 0
			if hasAudio {
				channels = 1
				if input.AudioChannels != nil {
					channels = int(*input.AudioChannels)
				}
			}
			links := []xLink{}
			if input.Kind != "audio" {
				links = append(links, xLink{Ref: item.ID})
			}
			for channel := 1; channel <= channels; channel++ {
				group := 0
				if channels == 2 {
					group = 1
				}
				links = append(links, xLink{Ref: item.ID + "-audio-" + strconv.Itoa(channel), Group: group})
			}
			item.Links = links
			if input.Kind != "audio" {
				video.Clips = append(video.Clips, item)
			}
			for channel := 1; channel <= channels; channel++ {
				if len(audio) < channel {
					audio = append(audio, xTrack{Enabled: boolText(track.Visible && !track.Muted), Locked: boolText(track.Locked)})
				}
				sound := item
				sound.ID += "-audio-" + strconv.Itoa(channel)
				sound.Source = xSource{MediaType: "audio", TrackIndex: channel}
				sound.Enabled = boolText(track.Visible && !track.Muted && clip.Volume > 0)
				sound.Filter = &xFilter{Effect: xEffect{Name: "Audio Levels", ID: "audiolevels", Category: "audiolevels", Type: "audio", MediaType: "audio", Parameter: xParameter{ID: "level", Name: "Level", Min: 0, Max: 4, Value: clip.Volume}}}
				audio[channel-1].Clips = append(audio[channel-1].Clips, sound)
			}
		}
		if len(video.Clips) > 0 {
			seq.Media.Video.Tracks = append(seq.Media.Video.Tracks, video)
		}
		seq.Media.Audio.Tracks = append(seq.Media.Audio.Tracks, audio...)
	}
	return xmlBytes(xmeml{Version: "5", Sequence: seq}, "xmeml")
}
