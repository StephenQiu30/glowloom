package managed

import (
	"context"
	"encoding/binary"
	"testing"
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
	output, duration, err := normalizeMusicAudio(context.Background(), input)
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
	if _, _, err := normalizeMusicAudio(context.Background(), input); err == nil {
		t.Fatal("invalid bytes accepted as music")
	}
}
