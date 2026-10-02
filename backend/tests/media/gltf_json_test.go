package media_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/StephenQiu30/lanverse/backend/internal/media/adapter/gltf"
	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func gltfJSONDocument(t *testing.T) map[string]any {
	t.Helper()
	doc := glbDocument()
	doc["buffers"].([]any)[0].(map[string]any)["uri"] = "data:application/octet-stream;base64," + base64.StdEncoding.EncodeToString(glbGeometry())
	doc["images"] = []any{map[string]any{"uri": "data:image/png;base64," + base64.StdEncoding.EncodeToString(uploadPNG(t))}}
	return doc
}

func gltfJSONBytes(t *testing.T, doc map[string]any) []byte {
	t.Helper()
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGLTFJSONValidatorChecksActualSelfContainedGeometryAndImages(t *testing.T) {
	validator, err := gltf.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	valid := gltfJSONBytes(t, gltfJSONDocument(t))
	if err := validator.ValidateJSON(t.Context(), bytes.NewReader(valid), int64(len(valid))); err != nil {
		t.Fatal("actual self-contained glTF JSON rejected", err)
	}
	// JSON originals are not subject to GLB's 4 MiB JSON chunk budget.
	padded := append(bytes.Repeat([]byte(" "), 5<<20), valid...)
	if err := validator.ValidateJSON(t.Context(), bytes.NewReader(padded), int64(len(padded))); err != nil {
		t.Fatal("bounded JSON original was incorrectly treated as a GLB chunk", err)
	}
	for _, uri := range []string{"https://example.invalid/private.bin", "http://127.0.0.1/resource", "file:///private.bin", "geometry.bin", "../geometry.bin", "data:application/octet-stream;base64,AAA="} {
		doc := gltfJSONDocument(t)
		doc["buffers"].([]any)[0].(map[string]any)["uri"] = uri
		body := gltfJSONBytes(t, doc)
		if err := validator.ValidateJSON(t.Context(), bytes.NewReader(body), int64(len(body))); !errors.Is(err, gltf.ErrInvalidModel) {
			t.Fatal("uncontained resource or incorrect actual buffer length accepted", uri, err)
		}
	}
	for _, change := range []func(map[string]any){
		func(d map[string]any) { delete(d["buffers"].([]any)[0].(map[string]any), "uri") },
		func(d map[string]any) { d["images"].([]any)[0].(map[string]any)["uri"] = "texture.png" },
		func(d map[string]any) { d["images"].([]any)[0].(map[string]any)["uri"] = "data:image/png;base64,AAAA" },
		func(d map[string]any) { d["accessors"].([]any)[0].(map[string]any)["count"] = 4 },
		func(d map[string]any) { d["extensionsRequired"] = []string{"KHR_draco_mesh_compression"} },
	} {
		doc := gltfJSONDocument(t)
		change(doc)
		body := gltfJSONBytes(t, doc)
		if err := validator.ValidateJSON(t.Context(), bytes.NewReader(body), int64(len(body))); !errors.Is(err, gltf.ErrInvalidModel) {
			t.Fatal("invalid actual glTF resource accepted", err)
		}
	}
	for _, body := range [][]byte{valid[:len(valid)-1], []byte(`{"asset":{"version":"2.0"},"asset":{"version":"2.0"}}`), glbBytes(t, glbDocument(), glbGeometry())} {
		if err := validator.ValidateJSON(t.Context(), bytes.NewReader(body), int64(len(body))); !errors.Is(err, gltf.ErrInvalidModel) {
			t.Fatal("bad JSON or GLB renamed as JSON accepted", err)
		}
	}
	if err := validator.ValidateJSON(t.Context(), bytes.NewReader(valid), gltf.MaxBytes+1); !errors.Is(err, gltf.ErrInvalidModel) {
		t.Fatal("oversize JSON accepted", err)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	if err := validator.ValidateJSON(cancelled, bytes.NewReader(valid), int64(len(valid))); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled offline validation ignored context", err)
	}
}

func TestGLTFJSONReadUploadAndProbeKeepOriginalFormat(t *testing.T) {
	body := gltfJSONBytes(t, gltfJSONDocument(t))
	file, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(body), "自包含场景.gltf")
	if err != nil {
		t.Fatal("source JSON model format missing from formal upload", err)
	}
	defer func() { _ = file.Close() }()
	if file.MIMEType != "model/gltf+json" || file.Size != int64(len(body)) {
		t.Fatal("JSON model misclassified or rewritten", file.MIMEType, file.Size)
	}
	validator, err := gltf.NewValidator()
	if err != nil {
		t.Fatal(err)
	}
	probe, err := mediaflow.NewUploadProber(validator).Probe(t.Context(), file)
	if err != nil || probe.Kind != domain.KindModel || probe.Extension != "gltf" || probe.Codec == nil || *probe.Codec != "gltf2" || probe.Width != nil || probe.Height != nil || probe.DurationMS != nil {
		t.Fatal("JSON model did not receive actual formal facts", probe, err)
	}
	if _, err := (mediaflow.FFUploadProber{}).Probe(t.Context(), file); !errors.Is(err, mediaapp.ErrUnavailable) {
		t.Fatal("missing offline validator accepted model", err)
	}
	if _, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(body), "false.glb"); !errors.Is(err, mediaapp.ErrUnsupportedUpload) {
		t.Fatal("JSON was accepted by changing its extension", err)
	}
}
