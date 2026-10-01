package canvas_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/StephenQiu30/lanverse/backend/internal/platform/objectstorage"
)

type browserMedia struct {
	kind, extension, name, mime string
	content                     []byte
	width, height, durationMS   *int32
}

func ownedBrowserMedia(t *testing.T) []browserMedia {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 160, 90))
	for y := range 90 {
		for x := range 160 {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y * 2), B: 180, A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal("encode owned image fixture")
	}
	// This tracked synthetic fixture belongs to backend tests, never a user's media.
	video, err := os.ReadFile("testdata/preview.mp4")
	if err != nil || len(video) == 0 || len(video) > 16<<20 {
		t.Fatal("tracked synthetic video fixture unavailable")
	}
	const sampleRate, sampleCount = 8000, 16000
	wav := make([]byte, 44+sampleCount*2)
	copy(wav[0:4], "RIFF")
	binary.LittleEndian.PutUint32(wav[4:8], uint32(len(wav)-8))
	copy(wav[8:16], "WAVEfmt ")
	binary.LittleEndian.PutUint32(wav[16:20], 16)
	binary.LittleEndian.PutUint16(wav[20:22], 1) // PCM
	binary.LittleEndian.PutUint16(wav[22:24], 1) // mono
	binary.LittleEndian.PutUint32(wav[24:28], sampleRate)
	binary.LittleEndian.PutUint32(wav[28:32], sampleRate*2)
	binary.LittleEndian.PutUint16(wav[32:34], 2)
	binary.LittleEndian.PutUint16(wav[34:36], 16)
	copy(wav[36:40], "data")
	binary.LittleEndian.PutUint32(wav[40:44], sampleCount*2)
	for i := range sampleCount {
		sample := int16(1800 * math.Sin(2*math.Pi*440*float64(i)/sampleRate))
		binary.LittleEndian.PutUint16(wav[44+i*2:46+i*2], uint16(sample))
	}
	return []browserMedia{
		{kind: "image", extension: "png", name: "Canvas-owned-image.png", mime: "image/png", content: encoded.Bytes(), width: ptr(int32(160)), height: ptr(int32(90))},
		{kind: "video", extension: "mp4", name: "Canvas-owned-video.mp4", mime: "video/mp4", content: video, width: ptr(int32(320)), height: ptr(int32(180)), durationMS: ptr(int32(2000))},
		{kind: "audio", extension: "wav", name: "Canvas-owned-audio.wav", mime: "audio/wav", content: wav, durationMS: ptr(int32(2000))},
	}
}

func seedOwnedCanvasBrowserMedia(t *testing.T, database *gorm.DB, storage *objectstorage.Client, project uuid.UUID) map[uuid.UUID][32]byte {
	t.Helper()
	digests := make(map[uuid.UUID][32]byte)
	for _, media := range ownedBrowserMedia(t) {
		id := uuid.New()
		// Both project and asset IDs were created by this test. Canonical media
		// keys preserve domain validation without claiming any existing object.
		key := "projects/" + project.String() + "/" + media.kind + "/" + time.Now().UTC().Format("2006/01") + "/" + id.String() + "." + media.extension
		t.Cleanup(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := storage.Remove(ctx, key); err != nil {
				t.Error("remove exact owned canvas media object failed")
			}
		})
		digest := sha256.Sum256(media.content)
		sha := hex.EncodeToString(digest[:])
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		err := storage.PutIfAbsent(ctx, key, bytes.NewReader(media.content), int64(len(media.content)), media.mime, sha)
		cancel()
		if err != nil {
			t.Fatal("write owned canvas media object failed")
		}
		if err := database.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,file_name,mime_type,byte_size,sha256,width,height,duration_ms,moderation_status) VALUES(?,?,?,'upload','ready',?,?,?,?,?,?,?,?,'passed')`, id, project, media.kind, key, media.name, media.mime, len(media.content), sha, media.width, media.height, media.durationMS).Error; err != nil {
			t.Fatal("persist owned canvas media fixture failed")
		}
		digests[id] = digest
	}
	return digests
}

func TestOwnedCanvasMediaContents(t *testing.T) {
	media := ownedBrowserMedia(t)
	if len(media) != 3 {
		t.Fatal("missing media fixtures")
	}
	img, err := png.Decode(bytes.NewReader(media[0].content))
	if err != nil || img.Bounds().Dx() != 160 || img.Bounds().Dy() != 90 {
		t.Fatal("invalid playable PNG fixture")
	}
	wav := media[2].content
	if len(wav) != 32044 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" || binary.LittleEndian.Uint32(wav[40:44]) != 32000 {
		t.Fatal("invalid playable PCM WAV fixture")
	}
}
