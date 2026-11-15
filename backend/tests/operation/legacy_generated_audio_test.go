package operation_test

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io"
	"os"
	"testing"

	artifact "github.com/StephenQiu30/glowloom-server/backend/internal/contract/providerartifact"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/fetch"
	mediaflow "github.com/StephenQiu30/glowloom-server/backend/media/adapter/workflow"
)

func TestNormalizeMusicWAVProducesVerifiedMP3(t *testing.T) {
	// A one-second, mono PCM WAV fixture exercises the real local decoder and
	// encoder. No provider, network, private recording, or API credential is used.
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
	input, err := tempArtifact(data, "audio/wave")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = input.Close() }()
	output, duration, err := normalizeMusicAudio(t.Context(), input)
	if err != nil || len(output) == 0 || duration < 1000 || duration > 1200 || string(output[:4]) == "RIFF" {
		t.Fatalf("normalization: bytes=%d duration=%d err=%v", len(output), duration, err)
	}
}

func TestNormalizeMusicRejectsNonMedia(t *testing.T) {
	input, err := tempArtifact([]byte("not an audio stream"), "audio/wave")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = input.Close() }()
	if _, _, err := normalizeMusicAudio(t.Context(), input); err == nil {
		t.Fatal("invalid bytes accepted as music")
	}
}

func tempArtifact(data []byte, mime string) (*fetch.Downloaded, error) {
	file, err := os.CreateTemp("", "glowloom-generated-audio-test-*")
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(data)
	input := &fetch.Downloaded{File: file, Size: int64(len(data)), MIMEType: mime, SHA256: hex.EncodeToString(digest[:])}
	if _, err := file.Write(data); err != nil {
		_ = input.Close()
		return nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		_ = input.Close()
		return nil, err
	}
	return input, nil
}

func normalizeMusicAudio(ctx context.Context, input *fetch.Downloaded) ([]byte, int64, error) {
	request := artifact.Request{ByteSize: input.Size, SHA256: input.SHA256, MIME: input.MIMEType, Processing: "music_mp3"}
	output, duration, err := (mediaflow.ProviderArtifactProcessor{}).ProcessProviderArtifact(ctx, request, input)
	if err != nil {
		return nil, 0, err
	}
	if output != input {
		defer func() { _ = output.Close() }()
	}
	data, err := io.ReadAll(output.File)
	return data, duration, err
}
