package ffmpeg

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	mediadomain "github.com/StephenQiu30/lanverse/backend/internal/media/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

const maxTranscriptionPCM int64 = 512 << 20

// AudioPreprocessor owns bounded native FFmpeg decoding and verifies actual PCM samples.
type AudioPreprocessor struct{ prober mediaapp.Prober }

// NewAudioPreprocessor injects byte probing without importing another media adapter.
func NewAudioPreprocessor(prober mediaapp.Prober) (*AudioPreprocessor, error) {
	if prober == nil {
		return nil, application.ErrUnavailable
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("%w: FFmpeg unavailable", application.ErrUnavailable)
	}
	return &AudioPreprocessor{prober: prober}, nil
}

// Prepare decodes all actual audio until EOF and rejects outputs stopped by the byte budget.
func (p *AudioPreprocessor) Prepare(ctx context.Context, source *mediaapp.Downloaded, durationMS int64) (*application.PreparedAudio, error) {
	if p == nil || p.prober == nil {
		return nil, application.ErrUnavailable
	}
	if source == nil || source.File == nil || source.Size < 1 || source.Size > 500<<20 || durationMS < 1 || durationMS > 86400000 {
		return nil, application.ErrInvalidTranscription
	}
	exists, err := sourceHasAudio(ctx, source.File.Name())
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, application.ErrNoAudio
	}
	file, err := os.CreateTemp("", "lanverse-transcription-pcm-*.wav")
	if err != nil {
		return nil, err
	}
	output := &mediaapp.Downloaded{File: file, MIMEType: "audio/wav"}
	keep := false
	defer func() {
		if !keep {
			_ = output.Close()
		}
	}()
	cmd := exec.CommandContext(ctx, "ffmpeg", "-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-protocol_whitelist", "file,pipe", "-i", source.File.Name(), "-map", "0:a:0", "-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", "-f", "wav", "-fs", "536870912", file.Name())
	// Diagnostics can contain private paths or speech metadata and are not exposed.
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: PCM decoding failed", application.ErrInvalidTranscription)
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	output.Size = info.Size()
	// FFmpeg may round its last packet around -fs. A full data block close to the
	// limit is rejected rather than mistaking an intentionally truncated WAV for EOF.
	if output.Size < 44 || output.Size >= maxTranscriptionPCM-65536 {
		return nil, application.ErrInvalidTranscription
	}
	pcmDuration, err := pcmDuration(file, output.Size)
	if err != nil {
		return nil, err
	}
	if pcmDuration < 1 || pcmDuration > 86400000 || pcmDuration > durationMS+100 {
		return nil, application.ErrInvalidTranscription
	}
	probeFile := *output
	probeFile.MIMEType = "audio/x-wav"
	probe, err := p.prober.Probe(ctx, &probeFile)
	if err != nil || probe.Kind != mediadomain.KindAudio || probe.AudioChannels == nil || *probe.AudioChannels != 1 || probe.Codec == nil || *probe.Codec != "pcm_s16le" || probe.DurationMS == nil || math.Abs(float64(*probe.DurationMS)-float64(pcmDuration)) > 1 {
		return nil, application.ErrInvalidTranscription
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return nil, err
	}
	output.SHA256 = hex.EncodeToString(hash.Sum(nil))
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	keep = true
	return &application.PreparedAudio{File: output, DurationMS: pcmDuration}, nil
}

// pcmDuration checks the bounded RIFF structure, complete data length and exact
// native 16 kHz/mono/s16le layout before deriving duration from actual sample count.
func pcmDuration(file *os.File, size int64) (int64, error) {
	var header [12]byte
	if _, err := file.ReadAt(header[:], 0); err != nil || string(header[:4]) != "RIFF" || string(header[8:]) != "WAVE" || int64(binary.LittleEndian.Uint32(header[4:8]))+8 != size {
		return 0, application.ErrInvalidTranscription
	}
	formatOK := false
	var dataBytes int64
	offset := int64(12)
	for chunks := 0; offset < size && chunks < 64; chunks++ {
		var chunk [8]byte
		if _, err := file.ReadAt(chunk[:], offset); err != nil {
			return 0, application.ErrInvalidTranscription
		}
		length := int64(binary.LittleEndian.Uint32(chunk[4:]))
		offset += 8
		if length > size-offset {
			return 0, application.ErrInvalidTranscription
		}
		switch string(chunk[:4]) {
		case "fmt ":
			if formatOK || length != 16 {
				return 0, application.ErrInvalidTranscription
			}
			var format [16]byte
			if _, err := file.ReadAt(format[:], offset); err != nil {
				return 0, application.ErrInvalidTranscription
			}
			if binary.LittleEndian.Uint16(format[:]) != 1 || binary.LittleEndian.Uint16(format[2:]) != 1 || binary.LittleEndian.Uint32(format[4:]) != 16000 || binary.LittleEndian.Uint32(format[8:]) != 32000 || binary.LittleEndian.Uint16(format[12:]) != 2 || binary.LittleEndian.Uint16(format[14:]) != 16 {
				return 0, application.ErrInvalidTranscription
			}
			formatOK = true
		case "data":
			if dataBytes != 0 || !formatOK || length < 2 || length%2 != 0 {
				return 0, application.ErrInvalidTranscription
			}
			dataBytes = length
		}
		offset += length + length%2
	}
	if offset != size || !formatOK || dataBytes < 2 {
		return 0, application.ErrInvalidTranscription
	}
	return int64(math.Round(float64(dataBytes) * 1000 / 32000)), nil
}
