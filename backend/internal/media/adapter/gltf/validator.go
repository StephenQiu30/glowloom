// Package gltf validates bounded, self-contained GLB 2.0 uploads before storage.
package gltf

import (
	"bytes"
	"context"
	"embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	// MaxBytes bounds the original GLB and all of its contained resources.
	MaxBytes          int64 = 64 << 20
	maxJSONBytes            = 4 << 20
	maxNodes                = 4096
	maxAccessors            = 4096
	maxAccessorValues       = 8_000_000
	maxPrimitives           = 2048
	maxDrawVertices         = 3_000_000
	maxImages               = 128
	maxImagePixels    int64 = 40_000_000
	maxImageDimension       = 8192
	maxDepth                = 64
	schemaBase              = "https://lanverse.local/gltf/"
)

// ErrInvalidModel identifies an unsupported or malformed uploaded GLB.
var ErrInvalidModel = errors.New("invalid or unsupported GLB model")

//go:embed schema/*.json
var schemas embed.FS

// Validator keeps immutable, offline Khronos schemas. It performs no URI fetch.
type Validator struct {
	root       *jsonschema.Schema
	extensions map[string]*jsonschema.Schema
}

// NewValidator compiles bundled, unmodified schemas from Khronos glTF commit
// 5decc120c95764c319c4f92e7f7ead026d926ef3 without network access.
func NewValidator() (*Validator, error) {
	c := jsonschema.NewCompiler()
	files, err := fs.Glob(schemas, "schema/*.json")
	if err != nil {
		return nil, err
	}
	for _, name := range files {
		content, err := schemas.ReadFile(name)
		if err != nil {
			return nil, err
		}
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(content))
		if err != nil {
			return nil, fmt.Errorf("decode GLB schema: %w", err)
		}
		if err := c.AddResource(schemaBase+strings.TrimPrefix(name, "schema/"), document); err != nil {
			return nil, err
		}
	}
	root, err := c.Compile(schemaBase + "glTF.schema.json")
	if err != nil {
		return nil, fmt.Errorf("compile GLB schema: %w", err)
	}
	v := &Validator{root: root, extensions: make(map[string]*jsonschema.Schema)}
	for _, name := range files {
		base := strings.TrimPrefix(name, "schema/")
		if !strings.Contains(base, ".KHR_") {
			continue
		}
		compiled, err := c.Compile(schemaBase + base)
		if err != nil {
			return nil, fmt.Errorf("compile GLB extension schema: %w", err)
		}
		v.extensions[base] = compiled
	}
	return v, nil
}

// Validate checks native container, schema, actual resources, geometry and scene
// references before an upload is eligible for the owner's review and storage.
func (v *Validator) Validate(ctx context.Context, reader io.ReaderAt, size int64) error {
	if v == nil || v.root == nil || reader == nil || size < 20 || size > MaxBytes {
		return ErrInvalidModel
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	js, bin, err := readContainer(reader, size)
	if err != nil {
		return err
	}
	value, err := decodeUniqueJSON(js)
	if err != nil || v.root.Validate(value) != nil {
		return invalid("JSON schema")
	}
	if err := v.validateExtensions(value, "root", nil, 0); err != nil {
		return err
	}
	var doc document
	if err := json.Unmarshal(js, &doc); err != nil || doc.Asset.Version != "2.0" || doc.Asset.MinVersion != "" && doc.Asset.MinVersion != "2.0" {
		return invalid("asset version")
	}
	if len(doc.Nodes) > maxNodes || len(doc.Accessors) > maxAccessors || len(doc.Images) > maxImages || len(doc.Materials) > 512 || len(doc.Textures) > 256 || len(doc.BufferViews) > 8192 || len(doc.Buffers) > 128 || len(doc.Animations) > 128 || len(doc.Skins) > 256 {
		return invalid("resource budget")
	}
	buffers, err := doc.readBuffers(bin)
	if err != nil {
		return err
	}
	if err := doc.validateViews(buffers); err != nil {
		return err
	}
	values, err := doc.readAccessors(ctx, buffers)
	if err != nil {
		return err
	}
	if err := doc.validateImages(ctx, buffers); err != nil {
		return err
	}
	if err := doc.validateGeometry(values); err != nil {
		return err
	}
	return doc.validateScene(values)
}

func readContainer(reader io.ReaderAt, size int64) ([]byte, []byte, error) {
	var header [12]byte
	if _, err := reader.ReadAt(header[:], 0); err != nil || binary.LittleEndian.Uint32(header[:4]) != 0x46546c67 || binary.LittleEndian.Uint32(header[4:8]) != 2 || int64(binary.LittleEndian.Uint32(header[8:])) != size {
		return nil, nil, invalid("container header")
	}
	var js, bin []byte
	for offset, chunk := int64(12), 0; offset < size; chunk++ {
		var head [8]byte
		if size-offset < 8 {
			return nil, nil, invalid("chunk header")
		}
		if _, err := reader.ReadAt(head[:], offset); err != nil {
			return nil, nil, invalid("chunk header")
		}
		length, kind := int64(binary.LittleEndian.Uint32(head[:4])), binary.LittleEndian.Uint32(head[4:])
		offset += 8
		if length%4 != 0 || length > size-offset || chunk > 15 || chunk == 0 && (kind != 0x4e4f534a || length == 0 || length > maxJSONBytes) || chunk != 0 && kind == 0x4e4f534a || kind == 0x004e4942 && chunk != 1 {
			return nil, nil, invalid("chunk layout")
		}
		// GLB consumers ignore unrecognized chunks. Their bytes remain bounded
		// by the original cap and are never interpreted or loaded as resources.
		if chunk != 0 && kind != 0x004e4942 {
			offset += length
			continue
		}
		data := make([]byte, length)
		if _, err := reader.ReadAt(data, offset); err != nil {
			return nil, nil, invalid("chunk body")
		}
		if chunk == 0 {
			js = data
		} else {
			bin = data
		}
		offset += length
	}
	if js == nil {
		return nil, nil, invalid("missing JSON")
	}
	return js, bin, nil
}

func invalid(reason string) error { return fmt.Errorf("%w: %s", ErrInvalidModel, reason) }

// decodeUniqueJSON rejects duplicate keys and deep documents before schema work.
func decodeUniqueJSON(data []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	entries := 0
	var read func(int) (any, error)
	read = func(depth int) (any, error) {
		entries++
		if depth > maxDepth || entries > 65536 {
			return nil, ErrInvalidModel
		}
		token, err := d.Token()
		if err != nil {
			return nil, err
		}
		switch token {
		case json.Delim('{'):
			m := make(map[string]any)
			for d.More() {
				keyToken, err := d.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, ErrInvalidModel
				}
				if _, exists := m[key]; exists {
					return nil, ErrInvalidModel
				}
				value, err := read(depth + 1)
				if err != nil {
					return nil, err
				}
				m[key] = value
			}
			_, err := d.Token()
			return m, err
		case json.Delim('['):
			var result []any
			for d.More() {
				value, err := read(depth + 1)
				if err != nil {
					return nil, err
				}
				result = append(result, value)
			}
			_, err := d.Token()
			return result, err
		default:
			return token, nil
		}
	}
	result, err := read(0)
	if err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, ErrInvalidModel
	}
	return result, nil
}
