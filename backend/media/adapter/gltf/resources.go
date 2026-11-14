package gltf

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	_ "image/jpeg" // Register the native JPEG decoder for contained textures.
	_ "image/png"  // Register the native PNG decoder for contained textures.
	"io"
	"strings"

	_ "golang.org/x/image/webp" // Register the existing WebP decoder used by Three.
)

func dataURI(uri string, allowed map[string]bool) ([]byte, string, error) {
	metadata, encoded, ok := strings.Cut(uri, ",")
	if !ok || !strings.HasPrefix(metadata, "data:") || !strings.HasSuffix(metadata, ";base64") {
		return nil, "", invalid("external or unsupported URI")
	}
	mime := strings.TrimSuffix(strings.TrimPrefix(metadata, "data:"), ";base64")
	if !allowed[mime] || int64(base64.StdEncoding.DecodedLen(len(encoded))) > MaxBytes {
		return nil, "", invalid("data URI type or size")
	}
	data, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || len(data) == 0 {
		return nil, "", invalid("data URI bytes")
	}
	return data, mime, nil
}

func (d document) readBuffers(bin []byte) ([][]byte, error) {
	result := make([][]byte, len(d.Buffers))
	usedBin := false
	var total int64
	for i, buffer := range d.Buffers {
		if buffer.ByteLength < 1 || buffer.ByteLength > MaxBytes {
			return nil, invalid("buffer size")
		}
		var data []byte
		if buffer.URI == "" {
			if i != 0 || len(bin) == 0 || int64(len(bin)) < buffer.ByteLength || int64(len(bin)) > buffer.ByteLength+3 {
				return nil, invalid("BIN resource")
			}
			for _, padding := range bin[buffer.ByteLength:] {
				if padding != 0 {
					return nil, invalid("BIN padding")
				}
			}
			data, usedBin = bin[:buffer.ByteLength], true
		} else {
			var err error
			data, _, err = dataURI(buffer.URI, map[string]bool{"application/octet-stream": true, "application/gltf-buffer": true})
			if err != nil || int64(len(data)) != buffer.ByteLength {
				return nil, invalid("buffer URI length")
			}
		}
		total += int64(len(data))
		if total > MaxBytes {
			return nil, invalid("buffer budget")
		}
		result[i] = data
	}
	if len(bin) != 0 && !usedBin {
		return nil, invalid("unbound BIN chunk")
	}
	return result, nil
}

func (d document) validateViews(buffers [][]byte) error {
	for _, view := range d.BufferViews {
		if !index(view.Buffer, len(buffers)) || view.ByteOffset < 0 || view.ByteLength < 1 || view.ByteOffset > int64(len(buffers[view.Buffer])) || view.ByteLength > int64(len(buffers[view.Buffer]))-view.ByteOffset || view.ByteStride != 0 && (view.ByteStride < 4 || view.ByteStride > 252 || view.ByteStride%4 != 0) {
			return invalid("buffer view bounds")
		}
	}
	return nil
}

func (d document) viewBytes(buffers [][]byte, id int) ([]byte, error) {
	if !index(id, len(d.BufferViews)) {
		return nil, invalid("buffer view reference")
	}
	v := d.BufferViews[id]
	return buffers[v.Buffer][v.ByteOffset : v.ByteOffset+v.ByteLength], nil
}

func (d document) validateImages(ctx context.Context, buffers [][]byte) error {
	var pixels int64
	for _, item := range d.Images {
		if err := ctx.Err(); err != nil {
			return err
		}
		var data []byte
		mime := item.MIMEType
		if item.BufferView != nil {
			var err error
			data, err = d.viewBytes(buffers, *item.BufferView)
			if err != nil || d.BufferViews[*item.BufferView].ByteStride != 0 {
				return invalid("image view")
			}
		} else {
			var actual string
			var err error
			data, actual, err = dataURI(item.URI, map[string]bool{"image/png": true, "image/jpeg": true, "image/webp": true, "image/avif": true})
			if err != nil || mime != "" && actual != mime {
				return invalid("image URI")
			}
			mime = actual
		}
		if mime == "image/avif" {
			width, height, err := avifDimensions(data)
			if err != nil {
				return err
			}
			pixels += int64(width) * int64(height)
			if pixels > maxImagePixels {
				return invalid("decoded image budget")
			}
			if err := decodeAVIF(ctx, data, width, height); err != nil {
				return err
			}
			continue
		}
		cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil || format != "png" && format != "jpeg" && format != "webp" || mime != "image/"+format || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxImageDimension || cfg.Height > maxImageDimension {
			return invalid("image format or dimensions")
		}
		pixels += int64(cfg.Width) * int64(cfg.Height)
		if pixels > maxImagePixels {
			return invalid("decoded image budget")
		}
		decoded, actual, err := image.Decode(contextReader{ctx: ctx, reader: bytes.NewReader(data)})
		if err != nil || actual != format || decoded.Bounds().Dx() != cfg.Width || decoded.Bounds().Dy() != cfg.Height {
			return invalid("image decoding")
		}
	}
	for _, texture := range d.Textures {
		if !optionalIndex(texture.Source, len(d.Images)) || !optionalIndex(texture.Sampler, len(d.Samplers)) {
			return invalid("texture reference")
		}
		if texture.Source == nil && texture.Extensions["EXT_texture_webp"] == nil && texture.Extensions["EXT_texture_avif"] == nil {
			return invalid("texture missing source")
		}
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
