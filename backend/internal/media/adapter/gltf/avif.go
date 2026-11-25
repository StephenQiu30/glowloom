package gltf

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os/exec"
	"time"
)

// ErrDecoderUnavailable means the installed media tools cannot decode AVIF.
var ErrDecoderUnavailable = errors.New("AVIF decoder unavailable")

type isoBox struct {
	kind string
	data []byte
}

func isoBoxes(data []byte) ([]isoBox, error) {
	var result []isoBox
	for len(data) > 0 {
		if len(data) < 8 || len(result) > 4096 {
			return nil, invalid("AVIF box bounds")
		}
		size, head := uint64(binary.BigEndian.Uint32(data[:4])), uint64(8)
		if size == 1 {
			if len(data) < 16 {
				return nil, invalid("AVIF extended box")
			}
			size, head = binary.BigEndian.Uint64(data[8:16]), 16
		}
		if size == 0 {
			size = uint64(len(data))
		}
		if size < head || size > uint64(len(data)) {
			return nil, invalid("AVIF box size")
		}
		result = append(result, isoBox{kind: string(data[4:8]), data: data[head:size]})
		data = data[size:]
	}
	return result, nil
}

// avifDimensions resolves the primary item's ispe property before a real decode.
// It never searches arbitrary compressed bytes for dimensions or trusts MIME.
func avifDimensions(data []byte) (int, int, error) {
	boxes, err := isoBoxes(data)
	if err != nil {
		return 0, 0, err
	}
	var meta []byte
	brand := false
	for _, box := range boxes {
		switch box.kind {
		case "ftyp":
			if len(box.data) < 8 || len(box.data)%4 != 0 {
				return 0, 0, invalid("AVIF brand")
			}
			for i := 0; i < len(box.data); i += 4 {
				if i != 4 && string(box.data[i:i+4]) == "avif" {
					brand = true
				}
			}
		case "meta":
			if meta != nil || len(box.data) < 4 {
				return 0, 0, invalid("AVIF metadata")
			}
			meta = box.data[4:]
		}
	}
	if !brand || meta == nil {
		return 0, 0, invalid("AVIF container")
	}
	children, err := isoBoxes(meta)
	if err != nil {
		return 0, 0, err
	}
	var primary uint32
	var properties, associations []byte
	for _, box := range children {
		switch box.kind {
		case "pitm":
			if primary != 0 || len(box.data) < 6 {
				return 0, 0, invalid("AVIF primary item")
			}
			switch {
			case box.data[0] == 0:
				primary = uint32(binary.BigEndian.Uint16(box.data[4:6]))
			case box.data[0] == 1 && len(box.data) >= 8:
				primary = binary.BigEndian.Uint32(box.data[4:8])
			default:
				return 0, 0, invalid("AVIF primary version")
			}
		case "iprp":
			parts, err := isoBoxes(box.data)
			if err != nil {
				return 0, 0, err
			}
			for _, part := range parts {
				switch part.kind {
				case "ipco":
					if properties != nil {
						return 0, 0, invalid("AVIF duplicate properties")
					}
					properties = part.data
				case "ipma":
					if associations != nil {
						return 0, 0, invalid("AVIF duplicate associations")
					}
					associations = part.data
				}
			}
		}
	}
	if primary == 0 || properties == nil || associations == nil {
		return 0, 0, invalid("AVIF item properties")
	}
	parts, err := isoBoxes(properties)
	if err != nil {
		return 0, 0, err
	}
	for _, box := range parts {
		if box.kind != "ispe" {
			continue
		}
		if len(box.data) != 12 {
			return 0, 0, invalid("AVIF spatial property")
		}
		width, height := binary.BigEndian.Uint32(box.data[4:8]), binary.BigEndian.Uint32(box.data[8:12])
		if width < 1 || height < 1 || width > maxImageDimension || height > maxImageDimension || int64(width)*int64(height) > maxImagePixels {
			return 0, 0, invalid("AVIF auxiliary dimensions")
		}
	}
	ids, err := primaryProperties(associations, primary)
	if err != nil {
		return 0, 0, err
	}
	for _, id := range ids {
		if id < 1 || id > len(parts) {
			return 0, 0, invalid("AVIF property reference")
		}
		box := parts[id-1]
		if box.kind != "ispe" {
			continue
		}
		if len(box.data) != 12 {
			return 0, 0, invalid("AVIF spatial property")
		}
		width, height := int(binary.BigEndian.Uint32(box.data[4:8])), int(binary.BigEndian.Uint32(box.data[8:12]))
		if width < 1 || height < 1 || width > maxImageDimension || height > maxImageDimension || int64(width)*int64(height) > maxImagePixels {
			return 0, 0, invalid("AVIF dimensions")
		}
		return width, height, nil
	}
	return 0, 0, invalid("AVIF primary dimensions")
}

func primaryProperties(data []byte, primary uint32) ([]int, error) {
	if len(data) < 8 || data[0] > 1 {
		return nil, invalid("AVIF association header")
	}
	version, wide := data[0], data[3]&1 != 0
	entries := binary.BigEndian.Uint32(data[4:8])
	if entries > 4096 {
		return nil, invalid("AVIF association budget")
	}
	data = data[8:]
	var selected []int
	for range entries {
		idBytes := 2
		if version == 1 {
			idBytes = 4
		}
		if len(data) < idBytes+1 {
			return nil, invalid("AVIF association bytes")
		}
		var id uint32
		if version == 0 {
			id = uint32(binary.BigEndian.Uint16(data[:2]))
		} else {
			id = binary.BigEndian.Uint32(data[:4])
		}
		count := int(data[idBytes])
		data = data[idBytes+1:]
		for range count {
			bytes := 1
			if wide {
				bytes = 2
			}
			if len(data) < bytes {
				return nil, invalid("AVIF association index")
			}
			property := int(data[0] & 0x7f)
			if wide {
				property = int(binary.BigEndian.Uint16(data[:2]) & 0x7fff)
			}
			if id == primary && property != 0 {
				selected = append(selected, property)
			}
			data = data[bytes:]
		}
	}
	if len(data) != 0 || len(selected) == 0 {
		return nil, invalid("AVIF associations")
	}
	return selected, nil
}

// decodeAVIF uses the installed real decoder on contained bytes only. No file,
// URL or network protocol is available to FFmpeg; stdout has an exact pixel cap.
func decodeAVIF(ctx context.Context, data []byte, width, height int) error {
	bin, err := exec.LookPath("ffmpeg")
	if err != nil {
		return ErrDecoderUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "-nostdin", "-hide_banner", "-loglevel", "error", "-max_alloc", "167772160", "-protocol_whitelist", "pipe", "-f", "mov", "-i", "pipe:0", "-map", "0:v:0", "-frames:v", "1", "-pix_fmt", "rgba", "-f", "rawvideo", "pipe:1")
	cmd.Stdin = bytes.NewReader(data)
	output := &pixelCounter{limit: int64(width) * int64(height) * 4}
	cmd.Stdout, cmd.Stderr = output, io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return invalid("AVIF decode")
	}
	if output.size != output.limit {
		return invalid("AVIF decoded dimensions")
	}
	return nil
}

type pixelCounter struct{ size, limit int64 }

func (w *pixelCounter) Write(data []byte) (int, error) {
	if int64(len(data)) > w.limit-w.size {
		return 0, invalid("AVIF decoded pixel budget")
	}
	w.size += int64(len(data))
	return len(data), nil
}
