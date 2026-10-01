package ffmpeg

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	canvasdomain "github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

type captionReader struct {
	ctx                       context.Context
	config                    canvasdomain.TimelineConfig
	face                      font.Face
	width, height             int
	startFrame, frame, frames int64
	data                      []byte
	offset                    int
	key                       string
}

func (r *captionReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) == 0 {
		return 0, nil
	}
	if r.offset == len(r.data) {
		if r.frame >= r.frames {
			return 0, io.EOF
		}
		ms := (r.startFrame + r.frame) * 1000 / int64(r.config.FPS)
		text, key := captionText(r.config, ms)
		if len(r.data) == 0 || key != r.key {
			image, err := drawCaption(r.face, r.config.SubtitleStyle, text, r.width, r.height)
			if err != nil {
				return 0, err
			}
			var data bytes.Buffer
			if err := png.Encode(&data, image); err != nil {
				return 0, err
			}
			r.data, r.key = data.Bytes(), key
		}
		r.offset = 0
		r.frame++
	}
	n := copy(p, r.data[r.offset:])
	r.offset += n
	return n, nil
}

func captionText(config canvasdomain.TimelineConfig, ms int64) (string, string) {
	var text, key strings.Builder
	for _, track := range config.Tracks {
		if !track.Visible || track.Kind != "text" && track.Kind != "subtitle" {
			continue
		}
		for _, clip := range config.Clips {
			if clip.TrackID != track.ID || ms < clip.StartMS || ms >= clip.StartMS+clip.DurationMS || strings.TrimSpace(clip.Text) == "" {
				continue
			}
			if text.Len() > 0 {
				text.WriteString("\n")
			}
			text.WriteString(clip.Text)
			key.WriteString(clip.ID.String())
		}
	}
	return text.String(), key.String()
}

func drawCaption(face font.Face, style canvasdomain.SubtitleStyle, text string, width, height int) (*image.RGBA, error) {
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	if text == "" {
		return canvas, nil
	}
	drawer := font.Drawer{Dst: canvas, Face: face}
	lines := wrapCaption(&drawer, text, width-96)
	lineHeight := face.Metrics().Height.Ceil()
	if len(lines) > 32 || len(lines)*lineHeight > height-96 {
		return nil, fmt.Errorf("%w: caption does not fit the frame", application.ErrInvalidExport)
	}
	block := len(lines) * lineHeight
	y := 48
	switch style.Position {
	case "center":
		y = (height - block) / 2
	case "bottom":
		y = height - block - 48
	}
	hex, err := strconv.ParseUint(strings.TrimPrefix(style.Color, "#"), 16, 24)
	if err != nil {
		return nil, application.ErrInvalidExport
	}
	fill := image.NewUniform(color.RGBA{R: uint8(hex >> 16), G: uint8(hex >> 8), B: uint8(hex), A: 255})
	stroke := image.NewUniform(color.RGBA{A: 224})
	outline := max(1, style.FontSize/24)
	for _, line := range lines {
		x := (width - drawer.MeasureString(line).Ceil()) / 2
		baseline := y + face.Metrics().Ascent.Ceil()
		for _, offset := range []image.Point{{-outline, -outline}, {0, -outline}, {outline, -outline}, {-outline, 0}, {outline, 0}, {-outline, outline}, {0, outline}, {outline, outline}} {
			drawer.Src = stroke
			drawer.Dot = fixed.P(x+offset.X, baseline+offset.Y)
			drawer.DrawString(line)
		}
		drawer.Src = fill
		drawer.Dot = fixed.P(x, baseline)
		drawer.DrawString(line)
		y += lineHeight
	}
	return canvas, nil
}

func wrapCaption(drawer *font.Drawer, text string, width int) []string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r", ""), "\t", "    ")
	var lines []string
	for _, paragraph := range strings.Split(text, "\n") {
		var line strings.Builder
		for _, r := range paragraph {
			if line.Len() > 0 && drawer.MeasureString(line.String()+string(r)).Ceil() > width {
				lines = append(lines, line.String())
				line.Reset()
			}
			line.WriteRune(r)
		}
		lines = append(lines, line.String())
	}
	return lines
}

func newCaptionReader(ctx context.Context, config canvasdomain.TimelineConfig, typeface *opentype.Font, width, height int, start, frames int64) (*captionReader, error) {
	face, err := opentype.NewFace(typeface, &opentype.FaceOptions{Size: float64(config.SubtitleStyle.FontSize), DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	return &captionReader{ctx: ctx, config: config, face: face, width: width, height: height, startFrame: start, frames: frames}, nil
}
