package media_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func TestAvatarDecodeLimits(t *testing.T) {
	var v bytes.Buffer
	if err := png.Encode(&v, image.NewNRGBA(image.Rect(0, 0, 2, 3))); err != nil {
		t.Fatal(err)
	}
	out, err := application.NormalizeAvatar(v.Bytes(), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if out.Width != 2 || out.Height != 3 || out.MIMEType != "image/png" || len(out.Bytes) == 0 {
		t.Fatal("wrong reencode")
	}
	var jpegBytes bytes.Buffer
	if err := jpeg.Encode(&jpegBytes, image.NewNRGBA(image.Rect(0, 0, 4, 5)), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := application.NormalizeAvatar(jpegBytes.Bytes(), "image/jpeg"); err != nil {
		t.Fatal("JPEG failed", err)
	}
	chunk := make([]byte, 20)
	binary.BigEndian.PutUint32(chunk, 8)
	copy(chunk[4:8], "acTL")
	binary.BigEndian.PutUint32(chunk[8:12], 2)
	binary.BigEndian.PutUint32(chunk[16:20], crc32.ChecksumIEEE(chunk[4:16]))
	apng := append(append(append([]byte{}, v.Bytes()[:33]...), chunk...), v.Bytes()[33:]...)
	if _, err := application.NormalizeAvatar(apng, "image/png"); err == nil {
		t.Fatal("APNG accepted")
	}
	var big bytes.Buffer
	if err := png.Encode(&big, image.NewNRGBA(image.Rect(0, 0, 4097, 1))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, mime string
		data       []byte
	}{{"fake", "image/png", []byte("fake")}, {"svg", "image/svg+xml", []byte(`<svg/>`)}, {"mime", "image/jpeg", v.Bytes()}, {"size", "image/png", make([]byte, 5*1024*1024+1)}, {"dimensions", "image/png", big.Bytes()}, {"animation", "image/gif", []byte("GIF89a")}} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := application.NormalizeAvatar(tc.data, tc.mime); err == nil {
				t.Fatal("invalid avatar accepted")
			}
		})
	}
}

func TestAvatarNormalizesCompressedHighDetailWithoutRejecting(t *testing.T) {
	// A deterministic high-entropy source is small as JPEG but expands past the
	// input byte cap when its decoded pixels are encoded directly to PNG.
	source := image.NewNRGBA(image.Rect(0, 0, 2048, 1536))
	state := uint32(793)
	for i := 0; i < len(source.Pix); i += 4 {
		state ^= state << 13
		state ^= state >> 17
		state ^= state << 5
		source.Pix[i], source.Pix[i+1], source.Pix[i+2], source.Pix[i+3] = byte(state), byte(state>>8), byte(state>>16), 255
	}
	var compressed bytes.Buffer
	if err := jpeg.Encode(&compressed, source, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	if compressed.Len() > application.AvatarMaxBytes {
		t.Fatal("invalid fixture: input exceeds accepted cap")
	}
	decoded, err := jpeg.Decode(bytes.NewReader(compressed.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	var expanded bytes.Buffer
	if err := png.Encode(&expanded, decoded); err != nil {
		t.Fatal(err)
	}
	if expanded.Len() <= application.AvatarMaxBytes {
		t.Fatal("fixture fails to exercise PNG expansion")
	}
	out, err := application.NormalizeAvatar(compressed.Bytes(), "image/jpeg")
	if err != nil {
		t.Fatal("legal compressed input rejected", err)
	}
	if out.Width != 1024 || out.Height != 768 {
		t.Fatalf("aspect ratio/size: %dx%d", out.Width, out.Height)
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(out.Bytes))
	if err != nil || cfg.Width != 1024 || cfg.Height != 768 {
		t.Fatal("normalized output invalid", err)
	}
	if len(out.Bytes) > 6*1024*1024 {
		t.Fatal("unbounded normalized output")
	}
}

func TestAvatarSmallImageNotUpscaledAndConvertedToEightBit(t *testing.T) {
	source := image.NewNRGBA64(image.Rect(0, 0, 13, 7))
	for i := range source.Pix {
		source.Pix[i] = byte(i * 71)
	}
	var input bytes.Buffer
	if err := png.Encode(&input, source); err != nil {
		t.Fatal(err)
	}
	out, err := application.NormalizeAvatar(input.Bytes(), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if out.Width != 13 || out.Height != 7 {
		t.Fatal("small image enlarged")
	}
	// The PNG IHDR bit-depth field must describe the explicit eight-bit output.
	if len(out.Bytes) < 25 || out.Bytes[24] != 8 {
		t.Fatal("avatar retained 16-bit channels")
	}
}
