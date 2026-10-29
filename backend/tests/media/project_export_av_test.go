package media_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	canvasdomain "github.com/StephenQiu30/glowloom-server/backend/internal/canvas/domain"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	project "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/adapter/editproject"
	tool "github.com/StephenQiu30/glowloom-server/backend/internal/mediatool/domain"
)

// Real generated video/audio originals validate rate conversion, trimming and stream facts.
func projectAVFixture(t *testing.T) (tool.FrozenExport, map[uuid.UUID]string) {
	t.Helper()
	frozen, paths := projectFixture(t)
	root := t.TempDir()
	commands := [][]string{
		{"-f", "lavfi", "-i", "color=c=blue:s=64x64:r=30000/1001:d=2", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-ac", "2", "-shortest", filepath.Join(root, "video.mp4")},
		{"-f", "lavfi", "-i", "sine=frequency=880:duration=2", "-c:a", "pcm_s16le", filepath.Join(root, "audio.wav")},
	}
	for i, args := range commands {
		if output, err := exec.CommandContext(t.Context(), "ffmpeg", append([]string{"-nostdin", "-hide_banner", "-loglevel", "error"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("real original generation %v: %s", err, output)
		}
		filePath := args[len(args)-1]
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.Open(filePath)
		if err != nil {
			t.Fatal(err)
		}
		mime, kind := "video/mp4", "video"
		if i == 1 {
			mime, kind = "audio/wave", "audio"
		}
		facts, err := (mediaflow.FFProber{}).Probe(t.Context(), &mediaapp.Downloaded{File: file, Size: int64(len(data)), MIMEType: mime})
		_ = file.Close()
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(data)
		id, trackID := uuid.New(), uuid.New()
		frozen.Inputs = append(frozen.Inputs, tool.FrozenSource{AssetID: id, Revision: 1, Kind: kind, ObjectKey: "private/" + kind, MIMEType: mime, ByteSize: int64(len(data)), SHA256: hex.EncodeToString(hash[:]), Width: facts.Width, Height: facts.Height, DurationMS: facts.DurationMS, FPS: facts.FPS, AudioChannels: facts.AudioChannels})
		paths[id] = filePath
		frozen.Timeline.Tracks = append(frozen.Timeline.Tracks, canvasdomain.TimelineTrack{ID: trackID, Kind: kind, Label: kind, Visible: true, Muted: i == 1, Locked: true})
		frozen.Timeline.Clips = append(frozen.Timeline.Clips, canvasdomain.TimelineClip{ID: uuid.New(), TrackID: trackID, Kind: kind, Title: kind, AssetID: &id, StartMS: 400, DurationMS: 800, SourceStartMS: 200, Volume: 0.5})
	}
	slices.SortFunc(frozen.Inputs, func(a, b tool.FrozenSource) int { return slices.Compare(a.AssetID[:], b.AssetID[:]) })
	return frozen, paths
}

func TestProjectExportVideoAudioTrimRateGainAndOriginals(t *testing.T) {
	frozen, paths := projectAVFixture(t)
	for _, kind := range []tool.OutputKind{tool.OutputFCPXML, tool.OutputXMEML} {
		t.Run(string(kind), func(t *testing.T) {
			frozen.OutputKind = kind
			out, err := project.NewRenderer(nil).Render(t.Context(), frozen, paths, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = out.Close() }()
			entries := archiveEntries(t, out.File, out.Size)
			if err = project.Verify(t.Context(), out.File, out.Size, frozen); err != nil {
				t.Fatal(err)
			}
			if kind == tool.OutputFCPXML {
				data := entries["timeline.fcpxmld/Info.fcpxml"]
				var doc struct {
					Clips []struct {
						Name     string `xml:"name,attr"`
						Start    string `xml:"start,attr"`
						Offset   string `xml:"offset,attr"`
						Duration string `xml:"duration,attr"`
						Enabled  string `xml:"enabled,attr"`
						Volume   struct {
							Amount string `xml:"amount,attr"`
						} `xml:"adjust-volume"`
					} `xml:"library>event>project>sequence>spine>gap>asset-clip"`
				}
				if err = xml.Unmarshal(data, &doc); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, clip := range doc.Clips {
					if clip.Name == "video" {
						found = true
						if clip.Start != "1001/5000s" || clip.Offset != "2/5s" || clip.Duration != "4/5s" || clip.Volume.Amount != "-6.02059991dB" {
							t.Fatalf("trim/gain %+v", clip)
						}
					}
					if clip.Name == "audio" && clip.Enabled != "0" {
						t.Fatal("muted audio became enabled")
					}
				}
				if !found || !strings.Contains(string(data), `frameDuration="1001/30000s"`) {
					t.Fatal("source NTSC rate lost")
				}
				if dtd := os.Getenv("GLOWLOOM_TEST_FCPXML_DTD"); dtd != "" {
					p := filepath.Join(t.TempDir(), "Info.fcpxml")
					if err = os.WriteFile(p, data, 0600); err != nil {
						t.Fatal(err)
					}
					if result, e := exec.CommandContext(t.Context(), "xmllint", "--nonet", "--noout", "--dtdvalid", dtd, p).CombinedOutput(); e != nil {
						t.Fatalf("AV DTD %v %s", e, result)
					}
				}
			} else {
				var doc struct {
					Video []struct {
						Name  string `xml:"name"`
						Start int    `xml:"start"`
						End   int    `xml:"end"`
						In    int    `xml:"in"`
						Out   int    `xml:"out"`
						NTSC  string `xml:"rate>ntsc"`
					} `xml:"sequence>media>video>track>clipitem"`
					Audio []struct {
						Name    string  `xml:"name"`
						Enabled string  `xml:"enabled"`
						Gain    float64 `xml:"filter>effect>parameter>value"`
						Channel int     `xml:"sourcetrack>trackindex"`
						Links   []struct {
							Ref   string `xml:"linkclipref"`
							Group int    `xml:"groupindex"`
						} `xml:"link"`
					} `xml:"sequence>media>audio>track>clipitem"`
				}
				if err = xml.Unmarshal(entries["timeline.xml"], &doc); err != nil {
					t.Fatal(err)
				}
				found := false
				for _, clip := range doc.Video {
					if clip.Name == "video" {
						found = true
						if clip.Start != 10 || clip.End != 30 || clip.In != 6 || clip.Out != 30 || clip.NTSC != "TRUE" {
							t.Fatalf("source/sequence frame conversion %+v", clip)
						}
					}
				}
				if !found || len(doc.Audio) != 3 {
					t.Fatalf("stream loss %+v", doc)
				}
				channels := []int{}
				for _, clip := range doc.Audio {
					if clip.Name == "video" {
						channels = append(channels, clip.Channel)
						groups := 0
						for _, link := range clip.Links {
							if link.Group == 1 {
								groups++
							}
						}
						if len(clip.Links) != 3 || groups != 2 {
							t.Fatalf("stereo links missing %+v", clip)
						}
					}
					if clip.Gain != 0.5 || clip.Name == "audio" && clip.Enabled != "FALSE" {
						t.Fatalf("audio gain/mute %+v", clip)
					}
				}
				if !slices.Equal(channels, []int{1, 2}) {
					t.Fatalf("stereo source channels lost %v", channels)
				}
			}
		})
	}
}
