package ffmpeg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"

	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

func (r *Renderer) renderAudio(ctx context.Context, frozen domain.FrozenExport, paths map[uuid.UUID]string, progress func(int, string) error) (*mediaapp.Downloaded, error) {
	parts, err := renderSegments(frozen.Timeline)
	if err != nil {
		return nil, err
	}
	tracks := make(map[uuid.UUID]canvasdomain.TimelineTrack, len(frozen.Timeline.Tracks))
	for _, track := range frozen.Timeline.Tracks {
		tracks[track.ID] = track
	}
	hasAudio := make(map[uuid.UUID]bool)
	audible := false
	for _, part := range parts {
		for _, clip := range part.clips {
			if !audibleClip(clip, tracks[clip.TrackID]) {
				continue
			}
			if clip.AssetID == nil || !filepath.IsAbs(paths[*clip.AssetID]) {
				return nil, application.ErrInvalidExport
			}
			if _, found := hasAudio[*clip.AssetID]; !found {
				stream, err := sourceHasAudio(ctx, paths[*clip.AssetID])
				if err != nil {
					return nil, err
				}
				hasAudio[*clip.AssetID] = stream
			}
			audible = audible || hasAudio[*clip.AssetID]
		}
	}
	if !audible {
		return nil, application.ErrNoAudio
	}
	root, err := os.MkdirTemp("", "lanverse-export-audio-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(root) }()
	var concat strings.Builder
	var stageBytes int64
	for i, part := range parts {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if progress != nil {
			if err := progress(20+i*60/len(parts), "rendering"); err != nil {
				return nil, err
			}
		}
		name := fmt.Sprintf("segment-%04d.wav", i)
		if err := renderAudioPart(ctx, root, name, frozen.Timeline, part, tracks, paths, hasAudio); err != nil {
			return nil, err
		}
		info, err := os.Stat(filepath.Join(root, name))
		if err != nil {
			return nil, err
		}
		if err := verifySegment(ctx, filepath.Join(root, name), part.end-part.start, frozen.Timeline.FPS); err != nil {
			return nil, err
		}
		stageBytes += info.Size()
		if stageBytes > maxStageBytes {
			return nil, fmt.Errorf("%w: audio export scratch budget", application.ErrInvalidExport)
		}
		fmt.Fprintf(&concat, "file '%s'\nduration %s\n", name, frameSeconds(part.end-part.start, frozen.Timeline.FPS))
	}
	if err := os.WriteFile(filepath.Join(root, "concat.txt"), []byte(concat.String()), 0600); err != nil {
		return nil, err
	}
	output, err := os.CreateTemp("", "lanverse-export-final-*.m4a")
	if err != nil {
		return nil, err
	}
	name := output.Name()
	_ = output.Close()
	keep := false
	defer func() {
		if !keep {
			_ = os.Remove(name)
		}
	}()
	if progress != nil {
		if err := progress(82, "encoding"); err != nil {
			return nil, err
		}
	}
	frames := parts[len(parts)-1].end
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-protocol_whitelist", "file,pipe", "-f", "concat", "-safe", "1", "-i", "concat.txt", "-map", "0:a", "-vn", "-t", frameSeconds(frames, frozen.Timeline.FPS), "-c:a", "aac", "-b:a", "128k", "-movflags", "+faststart", "-fs", strconv.FormatInt(maxExportBytes+1, 10), name}
	if err := run(ctx, root, args, nil); err != nil {
		return nil, err
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	result := &mediaapp.Downloaded{File: file, MIMEType: "audio/mp4"}
	defer func() {
		if !keep {
			_ = result.Close()
		}
	}()
	info, err := file.Stat()
	if err != nil || info.Size() < 1 || info.Size() > maxExportBytes {
		return nil, fmt.Errorf("%w: audio export output budget", application.ErrInvalidExport)
	}
	result.Size = info.Size()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return nil, err
	}
	result.SHA256 = hex.EncodeToString(hash.Sum(nil))
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	probe, err := r.prober.Probe(ctx, result)
	expected := frames * 1000 / int64(frozen.Timeline.FPS)
	if err != nil || probe.Kind != mediadomain.KindAudio || probe.Width != nil || probe.Height != nil || probe.AudioChannels == nil || *probe.AudioChannels != 2 || probe.DurationMS == nil || abs(int64(*probe.DurationMS)-expected) > int64(1000/frozen.Timeline.FPS)+25 {
		return nil, fmt.Errorf("%w: audio export output facts", application.ErrInvalidExport)
	}
	keep = true
	return result, nil
}

func audibleClip(clip canvasdomain.TimelineClip, track canvasdomain.TimelineTrack) bool {
	return track.Visible && !track.Muted && clip.Volume > 0 && (clip.Kind == "audio" || clip.Kind == "video")
}

func renderAudioPart(ctx context.Context, root, name string, config canvasdomain.TimelineConfig, part segment, tracks map[uuid.UUID]canvasdomain.TimelineTrack, paths map[uuid.UUID]string, audio map[uuid.UUID]bool) error {
	duration := frameSeconds(part.end-part.start, config.FPS)
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo:d=" + duration}
	filters := []string{"[0:a]asetpts=PTS-STARTPTS[a0]"}
	labels := []string{"[a0]"}
	for _, clip := range part.clips {
		if !audibleClip(clip, tracks[clip.TrackID]) || clip.AssetID == nil || !audio[*clip.AssetID] {
			continue
		}
		input := len(labels)
		elapsed := max(0, float64(part.start)*1000/float64(config.FPS)-float64(clip.StartMS))
		args = append(args, "-ss", seconds(float64(clip.SourceStartMS)+elapsed), "-protocol_whitelist", "file,pipe", "-t", duration, "-i", paths[*clip.AssetID])
		label := fmt.Sprintf("a%d", input)
		filters = append(filters, fmt.Sprintf("[%d:a]atrim=duration=%s,asetpts=PTS-STARTPTS,aresample=48000,aformat=channel_layouts=stereo,%s[%s]", input, duration, audioEnvelope(clip, elapsed), label))
		labels = append(labels, "["+label+"]")
	}
	filters = append(filters, strings.Join(labels, "")+fmt.Sprintf("amix=inputs=%d:duration=longest:normalize=0[aout]", len(labels)))
	args = append(args, "-filter_complex_threads", "1", "-filter_complex", strings.Join(filters, ";"), "-map", "[aout]", "-vn", "-t", duration, "-c:a", "pcm_s16le", "-fs", strconv.FormatInt(maxStageBytes+1, 10), name)
	return run(ctx, root, args, nil)
}
