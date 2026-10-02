package media_test

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"testing"

	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func uploadGIF(t *testing.T, frames int) []byte {
	t.Helper()
	animation := gif.GIF{LoopCount: 0, Config: image.Config{Width: 3, Height: 4}}
	for index := range frames {
		frame := image.NewPaletted(image.Rect(0, 0, 3, 4), color.Palette{color.Black, color.White})
		frame.SetColorIndex(index%3, index%4, 1)
		animation.Image = append(animation.Image, frame)
		animation.Delay = append(animation.Delay, 10)
	}
	var encoded bytes.Buffer
	if err := gif.EncodeAll(&encoded, &animation); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func TestGIFActualAnimationFullyDecodesAndRetainsOriginalFacts(t *testing.T) {
	data := uploadGIF(t, 3)
	file, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(data), "动画.gif")
	if err != nil {
		t.Fatal("actual animated GIF was not admitted by bytes", err)
	}
	defer func() { _ = file.Close() }()
	probe, err := (mediaflow.FFUploadProber{}).Probe(t.Context(), file)
	if err != nil || probe.Kind != domain.KindImage || probe.Extension != "gif" || probe.Codec == nil || *probe.Codec != "gif" || probe.Width == nil || *probe.Width != 3 || probe.Height == nil || *probe.Height != 4 || probe.DurationMS != nil {
		t.Fatal("GIF image facts were guessed or converted to video", probe, err)
	}
	if _, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(data), "renamed.png"); !errors.Is(err, mediaapp.ErrUnsupportedUpload) {
		t.Fatal("GIF extension renaming bypassed closed original type", err)
	}
}

func TestGIFRejectsLateFrameCorruptionBudgetsAndUnclaimedSuffix(t *testing.T) {
	valid := uploadGIF(t, 3)
	badLate := append([]byte(nil), valid...)
	badLate[len(badLate)-3] ^= 0xff
	tooWide := append([]byte(nil), valid...)
	tooWide[6], tooWide[7] = 1, 32 // 8193 logical width, rejected before allocating a canvas.
	for name, data := range map[string][]byte{
		"late compressed frame":    badLate,
		"too many frames":          uploadGIF(t, 1001),
		"oversized logical canvas": tooWide,
		"missing trailer":          valid[:len(valid)-1],
		"unclaimed suffix":         append(append([]byte(nil), valid...), []byte("unclaimed")...),
	} {
		t.Run(name, func(t *testing.T) {
			file, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(data), "invalid.gif")
			if err != nil {
				t.Fatal("GIF magic fixture reader", err)
			}
			defer func() { _ = file.Close() }()
			if _, err := (mediaflow.FFUploadProber{}).Probe(t.Context(), file); !errors.Is(err, mediaapp.ErrUnsupportedUpload) {
				t.Fatal("only the first GIF frame was validated", err)
			}
		})
	}
}
