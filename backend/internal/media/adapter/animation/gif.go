// Package animation validates the complete bounded GIF original before previews.
package animation

import (
	"bytes"
	"context"
	"encoding/binary"
	"image/gif"
	"io"

	"github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

const (
	maxGIFFrames        = 1000
	maxGIFDecodedPixels = 128_000_000
)

// ProbeGIF first bounds every frame's decoded allocation, then actually decodes
// all LZW streams. A valid first frame cannot hide a malformed later frame.
func ProbeGIF(ctx context.Context, file *application.Downloaded) (application.ProbeResult, error) {
	if file == nil || file.File == nil || file.MIMEType != "image/gif" || file.Size < 14 || file.Size > application.MaxUploadImageBytes {
		return application.ProbeResult{}, application.ErrUnsupportedUpload
	}
	data, err := io.ReadAll(gifContextReader{ctx, io.NewSectionReader(file.File, 0, file.Size+1)})
	if err != nil {
		return application.ProbeResult{}, err
	}
	if int64(len(data)) != file.Size {
		return application.ProbeResult{}, application.ErrUnsupportedUpload
	}
	width, height, frames, err := gifBudgets(ctx, data)
	if err != nil {
		return application.ProbeResult{}, err
	}
	decoded, err := gif.DecodeAll(gifContextReader{ctx, bytes.NewReader(data)})
	if err != nil || decoded.Config.Width != width || decoded.Config.Height != height || len(decoded.Image) != frames {
		if ctx.Err() != nil {
			return application.ProbeResult{}, ctx.Err()
		}
		return application.ProbeResult{}, application.ErrUnsupportedUpload
	}
	w, h, codec := int32(width), int32(height), "gif"
	return application.ProbeResult{Kind: domain.KindImage, Extension: "gif", Codec: &codec, Width: &w, Height: &h}, nil
}

func gifBudgets(ctx context.Context, data []byte) (int, int, int, error) {
	invalid := application.ErrUnsupportedUpload
	if len(data) < 14 || string(data[:6]) != "GIF87a" && string(data[:6]) != "GIF89a" {
		return 0, 0, 0, invalid
	}
	width, height := int(binary.LittleEndian.Uint16(data[6:8])), int(binary.LittleEndian.Uint16(data[8:10]))
	if width < 1 || height < 1 || width > 8192 || height > 8192 || int64(width)*int64(height) > 40_000_000 {
		return 0, 0, 0, invalid
	}
	offset := 13
	if data[10]&0x80 != 0 {
		offset += 3 << (1 + int(data[10]&7))
	}
	frames := 0
	var pixels int64
	for offset < len(data) {
		if err := ctx.Err(); err != nil {
			return 0, 0, 0, err
		}
		block := data[offset]
		offset++
		switch block {
		case 0x3b:
			if frames == 0 || offset != len(data) {
				return 0, 0, 0, invalid
			}
			return width, height, frames, nil
		case 0x21:
			if offset >= len(data) {
				return 0, 0, 0, invalid
			}
			offset++ // The native decoder validates the extension's actual contents.
		case 0x2c:
			if len(data)-offset < 9 {
				return 0, 0, 0, invalid
			}
			x, y := int(binary.LittleEndian.Uint16(data[offset:])), int(binary.LittleEndian.Uint16(data[offset+2:]))
			w, h := int(binary.LittleEndian.Uint16(data[offset+4:])), int(binary.LittleEndian.Uint16(data[offset+6:]))
			packed := data[offset+8]
			offset += 9
			frames++
			pixels += int64(w) * int64(h)
			if frames > maxGIFFrames || w < 1 || h < 1 || x > width-w || y > height-h || pixels > maxGIFDecodedPixels {
				return 0, 0, 0, invalid
			}
			if packed&0x80 != 0 {
				offset += 3 << (1 + int(packed&7))
			}
			if offset >= len(data) || data[offset] < 2 || data[offset] > 8 {
				return 0, 0, 0, invalid
			}
			offset++ // LZW minimum code size.
		default:
			return 0, 0, 0, invalid
		}
		// Both an extension and an image are sequences of bounded sub-blocks.
		for {
			if offset >= len(data) {
				return 0, 0, 0, invalid
			}
			length := int(data[offset])
			offset++
			if length == 0 {
				break
			}
			if length > len(data)-offset {
				return 0, 0, 0, invalid
			}
			offset += length
		}
	}
	return 0, 0, 0, invalid
}

type gifContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r gifContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
