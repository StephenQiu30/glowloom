package videodepth

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"

	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

// VerifyCanonicalOutput checks the fixed profile's actual MP4 stream/container.
func (p *Preprocessor) VerifyCanonicalOutput(ctx context.Context, path string) error {
	if p == nil {
		return application.ErrDepthRuntimeUnavailable
	}
	data, err := runCommand(ctx, p.ffprobe, []string{"-v", "error", "-protocol_whitelist", "file,pipe", "-show_entries", "stream=codec_name,codec_type,pix_fmt", "-of", "json", path}, maxProcessRSS)
	if err != nil {
		return errors.Join(application.ErrDepthOutputInvalid, err)
	}
	var value struct {
		Streams []struct {
			Codec string `json:"codec_name"`
			Kind  string `json:"codec_type"`
			Pixel string `json:"pix_fmt"`
		} `json:"streams"`
	}
	if json.Unmarshal(data, &value) != nil || len(value.Streams) != 1 || value.Streams[0].Kind != "video" || value.Streams[0].Codec != "h264" || value.Streams[0].Pixel != "yuv420p" {
		return application.ErrDepthOutputInvalid
	}
	return verifyFaststart(ctx, path)
}

func verifyFaststart(ctx context.Context, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return errors.Join(application.ErrDepthOutputInvalid, err)
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() < 16 || info.Size() > 500<<20 {
		return application.ErrDepthOutputInvalid
	}
	var offset int64
	var boxes int
	var moov, mdat bool
	for offset < info.Size() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		boxes++
		if boxes > 1024 || info.Size()-offset < 8 {
			return application.ErrDepthOutputInvalid
		}
		var header [16]byte
		if _, err := file.ReadAt(header[:8], offset); err != nil {
			return application.ErrDepthOutputInvalid
		}
		size := uint64(binary.BigEndian.Uint32(header[:4]))
		kind := string(header[4:8])
		headerSize := uint64(8)
		if size == 1 {
			if _, err := file.ReadAt(header[8:], offset+8); err != nil {
				return application.ErrDepthOutputInvalid
			}
			size = binary.BigEndian.Uint64(header[8:])
			headerSize = 16
		}
		if size < headerSize || size > uint64(info.Size()-offset) {
			return application.ErrDepthOutputInvalid
		}
		if offset == 0 {
			if kind != "ftyp" || size < headerSize+8 {
				return application.ErrDepthOutputInvalid
			}
			var brand [4]byte
			if _, err := file.ReadAt(brand[:], int64(headerSize)); err != nil || string(brand[:]) != "isom" {
				return application.ErrDepthOutputInvalid
			}
		}
		switch kind {
		case "moov":
			if moov || mdat {
				return application.ErrDepthOutputInvalid
			}
			moov = true
		case "mdat":
			if !moov {
				return application.ErrDepthOutputInvalid
			}
			mdat = true
		}
		offset += int64(size)
	}
	if !moov || !mdat {
		return application.ErrDepthOutputInvalid
	}
	return nil
}
