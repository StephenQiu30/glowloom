package media_test

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/media/adapter/gltf"
)

func glbDocument() map[string]any {
	return map[string]any{
		"asset": map[string]any{"version": "2.0"}, "scene": 0,
		"scenes":      []any{map[string]any{"nodes": []int{0}}},
		"nodes":       []any{map[string]any{"mesh": 0}},
		"meshes":      []any{map[string]any{"primitives": []any{map[string]any{"attributes": map[string]any{"POSITION": 0}}}}},
		"accessors":   []any{map[string]any{"bufferView": 0, "componentType": 5126, "count": 3, "type": "VEC3", "min": []int{0, 0, 0}, "max": []int{1, 1, 0}}},
		"bufferViews": []any{map[string]any{"buffer": 0, "byteLength": 36}},
		"buffers":     []any{map[string]any{"byteLength": 36}},
	}
}

func TestGLBValidatorPreservesThreeNativeExtensionsAndEmbeddedImages(t *testing.T) {
	validator, err := gltf.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"KHR_materials_unlit", "KHR_materials_clearcoat", "KHR_materials_dispersion", "KHR_materials_emissive_strength", "KHR_materials_ior", "KHR_materials_specular", "KHR_materials_transmission", "KHR_materials_iridescence", "KHR_materials_anisotropy", "KHR_materials_sheen", "KHR_materials_volume", "EXT_materials_bump"} {
		t.Run(name, func(t *testing.T) {
			d := glbDocument()
			d["extensionsUsed"], d["extensionsRequired"] = []string{name}, []string{name}
			d["materials"] = []any{map[string]any{"extensions": map[string]any{name: map[string]any{}}}}
			d["meshes"].([]any)[0].(map[string]any)["primitives"].([]any)[0].(map[string]any)["material"] = 0
			data := glbBytes(t, d, glbGeometry())
			if err := validator.Validate(t.Context(), bytes.NewReader(data), int64(len(data))); err != nil {
				t.Fatalf("native material rejected: %v", err)
			}
		})
	}
	t.Run("punctual light", func(t *testing.T) {
		d := glbDocument()
		d["extensionsUsed"] = []string{"KHR_lights_punctual"}
		d["extensions"] = map[string]any{"KHR_lights_punctual": map[string]any{"lights": []any{map[string]any{"type": "point", "intensity": 1}}}}
		d["nodes"].([]any)[0].(map[string]any)["extensions"] = map[string]any{"KHR_lights_punctual": map[string]any{"light": 0}}
		data := glbBytes(t, d, glbGeometry())
		if err := validator.Validate(t.Context(), bytes.NewReader(data), int64(len(data))); err != nil {
			t.Fatalf("native light rejected: %v", err)
		}
	})
	for _, tc := range []struct {
		name, mime, extension string
		image                 []byte
	}{
		{"PNG", "image/png", "", uploadPNG(t)},
		{"WebP", "image/webp", "EXT_texture_webp", decodeGLBWebP(t)},
		{"AVIF", "image/avif", "EXT_texture_avif", encodeGLBAVIF(t)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := glbDocument()
			d["images"] = []any{map[string]any{"uri": "data:" + tc.mime + ";base64," + base64.StdEncoding.EncodeToString(tc.image)}}
			if tc.extension != "" {
				d["extensionsUsed"], d["extensionsRequired"] = []string{tc.extension}, []string{tc.extension}
				d["textures"] = []any{map[string]any{"extensions": map[string]any{tc.extension: map[string]any{"source": 0}}}}
			}
			data := glbBytes(t, d, glbGeometry())
			if err := validator.Validate(t.Context(), bytes.NewReader(data), int64(len(data))); err != nil {
				t.Fatalf("native embedded image rejected: %v", err)
			}
		})
	}
}

func decodeGLBWebP(t *testing.T) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString("UklGRiQAAABXRUJQVlA4IBgAAAAwAQCdASoEAAQAAgA0JaQAA3AA/vv9UAA=")
	if err != nil {
		t.Fatal(err)
	}
	return data
}
func encodeGLBAVIF(t *testing.T) []byte {
	t.Helper()
	// A locally generated 64x64 black AVIF exercises actual decoding without
	// requiring an optional AV1 encoder on every test runner.
	data, err := base64.StdEncoding.DecodeString("AAAAIGZ0eXBhdmlmAAAAAGF2aWZtaWYxbWlhZk1BMUIAAAD5bWV0YQAAAAAAAAAvaGRscgAAAAAAAAAAcGljdAAAAAAAAAAAAAAAAFBpY3R1cmVIYW5kbGVyAAAAAA5waXRtAAAAAAABAAAAHmlsb2MAAAAARAAAAQABAAAAAQAAASEAAAAeAAAAKGlpbmYAAAAAAAEAAAAaaW5mZQIAAAAAAQAAYXYwMUNvbG9yAAAAAGppcHJwAAAAS2lwY28AAAAUaXNwZQAAAAAAAABAAAAAQAAAABBwaXhpAAAAAAMICAgAAAAMYXYxQ4EADAAAAAATY29scm5jbHgAAgACAAIAAAAAF2lwbWEAAAAAAAAAAQABBAECgwQAAAAmbWRhdAoLAgAABRV//Er5AEAyDxAArAIFFCCBAAADJP/MgA==")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGLBValidatorPreservesQuantizedAndInstancedGeometry(t *testing.T) {
	validator, err := gltf.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	t.Run("quantization", func(t *testing.T) {
		d := glbDocument()
		d["extensionsUsed"], d["extensionsRequired"] = []string{"KHR_mesh_quantization"}, []string{"KHR_mesh_quantization"}
		a := d["accessors"].([]any)[0].(map[string]any)
		a["componentType"] = 5123
		var bin bytes.Buffer
		for _, v := range []uint16{0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0} {
			_ = binary.Write(&bin, binary.LittleEndian, v)
		}
		d["bufferViews"].([]any)[0].(map[string]any)["byteLength"] = 24
		d["bufferViews"].([]any)[0].(map[string]any)["byteStride"] = 8
		d["buffers"].([]any)[0].(map[string]any)["byteLength"] = 24
		data := glbBytes(t, d, bin.Bytes())
		if err := validator.Validate(t.Context(), bytes.NewReader(data), int64(len(data))); err != nil {
			t.Fatalf("native quantized geometry=%v", err)
		}
	})
	t.Run("GPU instancing", func(t *testing.T) {
		d := glbDocument()
		d["extensionsUsed"], d["extensionsRequired"] = []string{"EXT_mesh_gpu_instancing"}, []string{"EXT_mesh_gpu_instancing"}
		d["nodes"].([]any)[0].(map[string]any)["extensions"] = map[string]any{"EXT_mesh_gpu_instancing": map[string]any{"attributes": map[string]any{"TRANSLATION": 1}}}
		d["accessors"] = append(d["accessors"].([]any), map[string]any{"bufferView": 1, "componentType": 5126, "count": 2, "type": "VEC3"})
		d["bufferViews"] = append(d["bufferViews"].([]any), map[string]any{"buffer": 0, "byteOffset": 36, "byteLength": 24})
		d["buffers"].([]any)[0].(map[string]any)["byteLength"] = 60
		bin := append(glbGeometry(), make([]byte, 24)...)
		data := glbBytes(t, d, bin)
		if err := validator.Validate(t.Context(), bytes.NewReader(data), int64(len(data))); err != nil {
			t.Fatalf("native instancing=%v", err)
		}
		d["accessors"].([]any)[1].(map[string]any)["count"] = 4097
		data = glbBytes(t, d, bin)
		if !errors.Is(validator.Validate(t.Context(), bytes.NewReader(data), int64(len(data))), gltf.ErrInvalidModel) {
			t.Fatal("unbounded instances accepted")
		}
	})
	t.Run("texture transform", func(t *testing.T) {
		d := glbDocument()
		d["extensionsUsed"] = []string{"KHR_texture_transform"}
		d["images"] = []any{map[string]any{"uri": "data:image/png;base64," + base64.StdEncoding.EncodeToString(uploadPNG(t))}}
		d["textures"] = []any{map[string]any{"source": 0}}
		d["materials"] = []any{map[string]any{"pbrMetallicRoughness": map[string]any{"baseColorTexture": map[string]any{"index": 0, "extensions": map[string]any{"KHR_texture_transform": map[string]any{"offset": []int{0, 0}, "scale": []int{1, 1}, "rotation": 0}}}}}}
		d["meshes"].([]any)[0].(map[string]any)["primitives"].([]any)[0].(map[string]any)["material"] = 0
		data := glbBytes(t, d, glbGeometry())
		if err := validator.Validate(t.Context(), bytes.NewReader(data), int64(len(data))); err != nil {
			t.Fatalf("native texture transform=%v", err)
		}
	})
}

func glbGeometry() []byte {
	var data bytes.Buffer
	for _, value := range []float32{0, 0, 0, 1, 0, 0, 0, 1, 0} {
		_ = binary.Write(&data, binary.LittleEndian, value)
	}
	return data.Bytes()
}

func glbBytes(t *testing.T, document map[string]any, bin []byte) []byte {
	t.Helper()
	js, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	for len(js)%4 != 0 {
		js = append(js, ' ')
	}
	for len(bin)%4 != 0 {
		bin = append(bin, 0)
	}
	var result bytes.Buffer
	for _, value := range []uint32{0x46546c67, 2, uint32(12 + 8 + len(js) + 8 + len(bin)), uint32(len(js)), 0x4e4f534a} {
		_ = binary.Write(&result, binary.LittleEndian, value)
	}
	result.Write(js)
	_ = binary.Write(&result, binary.LittleEndian, uint32(len(bin)))
	_ = binary.Write(&result, binary.LittleEndian, uint32(0x004e4942))
	result.Write(bin)
	return result.Bytes()
}

func TestGLBValidatorChecksClosedBoundedNativeResources(t *testing.T) {
	validator, err := gltf.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		edit func(map[string]any, *[]byte)
	}{
		{"external buffer", func(d map[string]any, _ *[]byte) {
			d["buffers"].([]any)[0].(map[string]any)["uri"] = "https://fixture.invalid/data.bin"
		}},
		{"external image", func(d map[string]any, _ *[]byte) { d["images"] = []any{map[string]any{"uri": "../texture.png"}} }},
		{"required compression", func(d map[string]any, _ *[]byte) {
			d["extensionsUsed"] = []string{"KHR_draco_mesh_compression"}
			d["extensionsRequired"] = []string{"KHR_draco_mesh_compression"}
		}},
		{"unknown required", func(d map[string]any, _ *[]byte) {
			d["extensionsUsed"] = []string{"vendor_future"}
			d["extensionsRequired"] = []string{"vendor_future"}
		}},
		{"buffer length", func(d map[string]any, _ *[]byte) { d["buffers"].([]any)[0].(map[string]any)["byteLength"] = 40 }},
		{"view overflow", func(d map[string]any, _ *[]byte) { d["bufferViews"].([]any)[0].(map[string]any)["byteOffset"] = 4 }},
		{"accessor overflow", func(d map[string]any, _ *[]byte) { d["accessors"].([]any)[0].(map[string]any)["count"] = 4 }},
		{"missing mesh ref", func(d map[string]any, _ *[]byte) { d["nodes"].([]any)[0].(map[string]any)["mesh"] = 1 }},
		{"node cycle", func(d map[string]any, _ *[]byte) { d["nodes"].([]any)[0].(map[string]any)["children"] = []int{0} }},
		{"nonfinite position", func(_ map[string]any, b *[]byte) {
			binary.LittleEndian.PutUint32((*b)[:4], math.Float32bits(float32(math.Inf(1))))
		}},
		{"fake embedded PNG", func(d map[string]any, _ *[]byte) {
			d["images"] = []any{map[string]any{"bufferView": 0, "mimeType": "image/png"}}
		}},
		{"schema wrong count", func(d map[string]any, _ *[]byte) { d["accessors"].([]any)[0].(map[string]any)["count"] = "3" }},
		{"sparse index out of range", func(d map[string]any, b *[]byte) {
			(*b)[0] = 3
			d["accessors"].([]any)[0].(map[string]any)["sparse"] = map[string]any{"count": 1, "indices": map[string]any{"bufferView": 0, "componentType": 5121}, "values": map[string]any{"bufferView": 0}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, b := glbDocument(), glbGeometry()
			tc.edit(d, &b)
			data := glbBytes(t, d, b)
			if err := validator.Validate(t.Context(), bytes.NewReader(data), int64(len(data))); !errors.Is(err, gltf.ErrInvalidModel) {
				t.Fatalf("invalid GLB accepted: %v", err)
			}
		})
	}
	data := glbBytes(t, glbDocument(), glbGeometry())
	if err := validator.Validate(t.Context(), bytes.NewReader(data), int64(len(data))); err != nil {
		t.Fatalf("valid native triangle rejected: %v", err)
	}
	unknown := append([]byte(nil), data...)
	unknown = append(unknown, 4, 0, 0, 0, 'T', 'E', 'S', 'T', 0, 0, 0, 0)
	binary.LittleEndian.PutUint32(unknown[8:12], uint32(len(unknown)))
	if err := validator.Validate(t.Context(), bytes.NewReader(unknown), int64(len(unknown))); err != nil {
		t.Fatalf("bounded noninterpreted GLB extension chunk rejected: %v", err)
	}
	for _, offset := range []int{0, 4, 8, 12, 16} {
		invalid := append([]byte(nil), data...)
		binary.LittleEndian.PutUint32(invalid[offset:offset+4], 1)
		if err := validator.Validate(t.Context(), bytes.NewReader(invalid), int64(len(invalid))); !errors.Is(err, gltf.ErrInvalidModel) {
			t.Fatalf("malformed header/chunk offset=%d accepted: %v", offset, err)
		}
	}
	if err := validator.Validate(t.Context(), bytes.NewReader(data), 64<<20+1); !errors.Is(err, gltf.ErrInvalidModel) {
		t.Fatalf("oversized model accepted: %v", err)
	}
}
