package microservices_test

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"testing"

	contract "github.com/StephenQiu30/glowloom-server/backend/internal/contract/providerartifact"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/fetch"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/media/adapter/workflow"
)

func providerMusicWAV() []byte {
	data := make([]byte, 44+16000*2)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 16000)
	binary.LittleEndian.PutUint32(data[28:], 32000)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], uint32(len(data)-44))
	return data
}

func TestProviderArtifactMediaProcessorMusicUsesActualCanonicalBytes(t *testing.T) {
	data := providerMusicWAV()
	file, err := os.CreateTemp("", "glowloom-provider-music-test-*.wav")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	source := &fetch.Downloaded{File: file, Size: int64(len(data)), MIMEType: "audio/wave", SHA256: hex.EncodeToString(sum[:])}
	defer func() { _ = source.Close() }()
	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}
	req := artifactRequest(t, data)
	req.MIME = "audio/wave"
	req.Processing = "music_mp3"
	req.Command.SHA256 = req.Digest()
	normalized, duration, err := (mediaflow.ProviderArtifactProcessor{}).ProcessProviderArtifact(context.Background(), req, source)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = normalized.Close() }()
	if normalized == source || normalized.MIMEType != "audio/mpeg" || normalized.Size <= 0 || normalized.Size > 32<<20 || normalized.SHA256 == source.SHA256 || duration < 1000 || duration > 1200 {
		t.Fatalf("source was relabelled instead of normalized: result=%+v duration=%d", normalized, duration)
	}
	if _, err := source.File.Stat(); err != nil {
		t.Fatal("processor took ownership of borrowed original", err)
	}
	req.Processing = "video_probe"
	req.Command.SHA256 = req.Digest()
	if output, _, err := (mediaflow.ProviderArtifactProcessor{}).ProcessProviderArtifact(t.Context(), req, source); err == nil {
		if output != source {
			_ = output.Close()
		}
		t.Fatal("WAV accepted as video")
	}
	if err := req.Validate(); err != nil && !errors.Is(err, contract.ErrInvalid) {
		t.Fatal(err)
	}
}
