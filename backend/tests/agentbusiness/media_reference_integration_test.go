package agentbusiness_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"image"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	agentdomain "github.com/StephenQiu30/glowloom-server/backend/internal/agent/domain"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

func referenceDOCX(t *testing.T) []byte {
	t.Helper()
	var body bytes.Buffer
	archive := zip.NewWriter(&body)
	for name, text := range map[string]string{
		"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels":         `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml":   `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>小说真实开头</w:t></w:r></w:p><w:p><w:r><w:t>完整结局</w:t></w:r></w:p></w:body></w:document>`,
	} {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, text); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return body.Bytes()
}

func TestMediaReferenceBusinessReadsActualDocumentsAndVideo(t *testing.T) {
	videoPath := filepath.Join(t.TempDir(), "reference.mp4")
	if output, err := exec.CommandContext(t.Context(), "ffmpeg", "-nostdin", "-v", "error", "-f", "lavfi", "-i", "testsrc=size=160x90:rate=10", "-t", "3", "-pix_fmt", "yuv420p", "-y", videoPath).CombinedOutput(); err != nil {
		t.Fatalf("video fixture: %s: %v", output, err)
	}
	video, err := os.ReadFile(videoPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, kind, mime string
		body             []byte
		images           int
		ending           string
	}{
		{"txt", "document", "text/plain", []byte("小说真实开头\n全文最后一行完整结局"), 0, "全文最后一行完整结局"},
		{"docx", "document", mediadomain.MIMEDOCX, referenceDOCX(t), 0, "完整结局"},
		{"video", "video", "video/mp4", video, 3, "full clip and audio have not been inspected"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			temporary := t.TempDir()
			t.Setenv("TMPDIR", temporary)
			db, boundary, _, _, grant := realCanvasBoundary(t)
			id, ref := referenceAssetFixture(t, db, grant, tc.name, tc.kind, tc.mime, tc.body)
			reads := 0
			boundary.WithMediaObjects(imageObjects(func(_ context.Context, got mediadomain.ObjectRef) (io.ReadCloser, error) {
				reads++
				if got != ref {
					t.Fatal("source identity changed")
				}
				return io.NopCloser(bytes.NewReader(tc.body)), nil
			}))
			router := gin.New()
			boundary.BusinessHandler(businessToken).Register(router)
			status, receipt, message := businessToolRequest(t, router, grant, "media_reference_inspect", "actual-"+tc.name, map[string]any{"asset_id": id, "revision": 1, "media_kind": tc.kind})
			if status != 200 {
				t.Fatal(status, message)
			}
			var result struct {
				Content []struct{ Type, Text, Data string }
			}
			if err := json.Unmarshal(receipt.Result, &result); err != nil {
				t.Fatal(err)
			}
			images := 0
			text := ""
			for _, part := range result.Content {
				text += part.Text
				if part.Type == "image" {
					images++
					data, err := base64.StdEncoding.DecodeString(part.Data)
					if err != nil || len(data) > 2<<20 {
						t.Fatal("invalid frame", err)
					}
					if _, format, err := image.Decode(bytes.NewReader(data)); err != nil || format != "jpeg" {
						t.Fatal("actual JPEG missing", err)
					}
				}
			}
			if images != tc.images || !strings.Contains(text, tc.ending) || reads != 1 {
				t.Fatal("actual complete content missing", images, text, reads)
			}
			if tc.kind == "video" && (!strings.Contains(text, "300 ms") || !strings.Contains(text, "1500 ms") || !strings.Contains(text, "2700 ms")) {
				t.Fatal("sample targets missing", text)
			}
			if entries, err := os.ReadDir(temporary); err != nil || len(entries) != 0 {
				t.Fatal("source or frame temporary files remain", entries, err)
			}
		})
	}
}

func TestMediaReferenceRejectsCorruptOriginalAndOversizedSource(t *testing.T) {
	temporary := t.TempDir()
	t.Setenv("TMPDIR", temporary)
	db, boundary, _, _, grant := realCanvasBoundary(t)
	id, _, body := imageFixture(t, db, grant)
	reads := 0
	boundary.WithMediaObjects(imageObjects(func(context.Context, mediadomain.ObjectRef) (io.ReadCloser, error) {
		reads++
		return io.NopCloser(bytes.NewReader(append(body, byte('x')))), nil
	}))
	router := gin.New()
	boundary.BusinessHandler(businessToken).Register(router)
	status, _, _ := businessToolRequest(t, router, grant, "media_reference_inspect", "corrupt-reference", map[string]any{"asset_id": id, "revision": 1, "media_kind": "image"})
	if status == 200 {
		t.Fatal("corrupt source accepted")
	}
	if err := db.Table("media.media_asset").Where("id=?", id).Updates(map[string]any{"kind": "video", "byte_size": 501 << 20}).Error; err != nil {
		t.Fatal(err)
	}
	status, _, _ = businessToolRequest(t, router, grant, "media_reference_inspect", "oversized-reference", map[string]any{"asset_id": id, "revision": 1, "media_kind": "video"})
	if status == 200 || reads != 1 {
		t.Fatal("oversized source read", status, reads)
	}
	if entries, err := os.ReadDir(temporary); err != nil || len(entries) != 0 {
		t.Fatal("failed read temporary file remains", entries, err)
	}
}

func TestMediaReferenceStopDuringReadCreatesNoReceiptOrTemporaryFiles(t *testing.T) {
	temporary := t.TempDir()
	t.Setenv("TMPDIR", temporary)
	db, boundary, _, actor, grant := realCanvasBoundary(t)
	id, _, body := imageFixture(t, db, grant)
	boundary.WithMediaObjects(imageObjects(func(ctx context.Context, _ mediadomain.ObjectRef) (io.ReadCloser, error) {
		if _, err := boundary.Store.Decide(ctx, actor, grant.Scope.ProjectID, grant.RootRunID, agentdomain.DecisionInput{RequestID: uuid.New(), Action: "stop", ExpectedAuthorizationRevision: 1}); err != nil {
			t.Fatal(err)
		}
		return io.NopCloser(bytes.NewReader(body)), nil
	}))
	router := gin.New()
	boundary.BusinessHandler(businessToken).Register(router)
	status, _, message := businessToolRequest(t, router, grant, "media_reference_inspect", "stopped-reference", map[string]any{"asset_id": id, "revision": 1, "media_kind": "image"})
	if status != 409 {
		t.Fatal(status, message)
	}
	var n int64
	if err := db.Raw(`SELECT count(*) FROM agent_business.call_receipt WHERE root_run_id=?`, grant.RootRunID).Scan(&n).Error; err != nil || n != 0 {
		t.Fatal("late reference receipt", n, err)
	}
	if entries, err := os.ReadDir(temporary); err != nil || len(entries) != 0 {
		t.Fatal("stopped source temporary files remain", entries, err)
	}
}

func TestMediaReferenceRequestCancellationClosesBlockedSource(t *testing.T) {
	temporary := t.TempDir()
	t.Setenv("TMPDIR", temporary)
	db, boundary, _, _, grant := realCanvasBoundary(t)
	id, _, body := imageFixture(t, db, grant)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	reader, writer := io.Pipe()
	defer func() { _ = writer.Close() }()
	writerDone := make(chan struct{})
	boundary.WithMediaObjects(imageObjects(func(context.Context, mediadomain.ObjectRef) (io.ReadCloser, error) {
		go func() {
			defer close(writerDone)
			_, _ = writer.Write(body[:1])
			// Cancel only after Read has consumed source bytes. The next Read
			// would block forever unless the request owner closes this source.
			cancel()
		}()
		return reader, nil
	}))
	router := gin.New()
	boundary.BusinessHandler(businessToken).Register(router)
	forceClose := time.AfterFunc(2*time.Second, func() { _ = writer.Close() })
	defer forceClose.Stop()
	start := time.Now()
	status, _, _ := businessToolRequest(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		router.ServeHTTP(w, r.WithContext(ctx))
	}), grant, "media_reference_inspect", "cancelled-reference", map[string]any{"asset_id": id, "revision": 1, "media_kind": "image"})
	<-writerDone
	if status == 200 || time.Since(start) >= time.Second {
		t.Fatal("cancelled source was not closed promptly", status)
	}
	var n int64
	if err := db.Raw(`SELECT count(*) FROM agent_business.call_receipt WHERE root_run_id=?`, grant.RootRunID).Scan(&n).Error; err != nil || n != 0 {
		t.Fatal("cancelled reference receipt", n, err)
	}
	if entries, err := os.ReadDir(temporary); err != nil || len(entries) != 0 {
		t.Fatal("cancelled source temporary files remain", entries, err)
	}
}

func TestMediaReferenceInspectRealSourceRevisionReplayAndRevoke(t *testing.T) {
	db, b, _, _, grant := realCanvasBoundary(t)
	id, ref, body := imageFixture(t, db, grant)
	reads := 0
	b.WithMediaObjects(imageObjects(func(_ context.Context, got mediadomain.ObjectRef) (io.ReadCloser, error) {
		reads++
		if got != ref {
			t.Fatal("physical source changed")
		}
		return io.NopCloser(bytes.NewReader(body)), nil
	}))
	router := gin.New()
	b.BusinessHandler(businessToken).Register(router)
	request := map[string]any{"asset_id": id, "revision": 1, "media_kind": "image"}
	status, receipt, message := businessToolRequest(t, router, grant, "media_reference_inspect", "inspected-reference", request)
	if status != http.StatusOK {
		t.Fatal(status, message)
	}
	var result struct {
		AssetID   string `json:"asset_id"`
		Revision  int64  `json:"revision"`
		MediaKind string `json:"media_kind"`
		Content   []struct{ Type, Data, MIMEType string }
	}
	if err := json.Unmarshal(receipt.Result, &result); err != nil || result.AssetID != id.String() || result.Revision != 1 || result.MediaKind != "image" || len(result.Content) != 1 {
		t.Fatal(string(receipt.Result), err)
	}
	jpeg, err := base64.StdEncoding.DecodeString(result.Content[0].Data)
	if err != nil {
		t.Fatal(err)
	}
	decoded, kind, err := image.Decode(bytes.NewReader(jpeg))
	if err != nil || kind != "jpeg" || decoded.Bounds().Dx() != 640 && decoded.Bounds().Dx() != 2 {
		t.Fatal("actual normalized image missing", kind, err)
	}
	status, replay, message := businessToolRequest(t, router, grant, "media_reference_inspect", "inspected-reference", request)
	if status != 200 || replay.ReceiptID != receipt.ReceiptID || reads != 1 {
		t.Fatal("replay reread source", status, reads, message)
	}
	if err := db.Exec(`UPDATE media.media_asset SET revision=2 WHERE id=?`, id).Error; err != nil {
		t.Fatal(err)
	}
	status, _, _ = businessToolRequest(t, router, grant, "media_reference_inspect", "inspected-reference", request)
	if status != 409 || reads != 1 {
		t.Fatal("stale frozen source replayed", status, reads)
	}
	if err := db.Exec(`UPDATE media.media_asset SET is_delete=true WHERE id=?`, id).Error; err != nil {
		t.Fatal(err)
	}
	status, _, _ = businessToolRequest(t, router, grant, "media_reference_inspect", "inspected-reference", request)
	if status == 200 || reads != 1 {
		t.Fatal("revoked source replayed", status, reads)
	}
}

func referenceAssetFixture(t *testing.T, db *gorm.DB, grant agentdomain.Grant, extension, kind, mime string, body []byte) (uuid.UUID, mediadomain.ObjectRef) {
	t.Helper()
	if extension == "video" {
		extension = "mp4"
	}
	sum := sha256.Sum256(body)
	digest := hex.EncodeToString(sum[:])
	id, location := uuid.New(), uuid.New()
	profile := "agent-reference-" + uuid.NewString()[:8]
	ref := mediadomain.ObjectRef{ProfileID: profile, Bucket: "agent-references", Key: "projects/" + grant.Scope.ProjectID.String() + "/" + kind + "/2026/11/" + id.String() + "." + extension}
	if err := db.Exec(`INSERT INTO media.storage_profile(id,provider,endpoint,signing_endpoint,region,bucket,addressing_style,resource_identity)VALUES(?,'cos','http://127.0.0.1','http://127.0.0.1','us-east-1',?,'',?)`, profile, ref.Bucket, agentdomain.ContentHash(profile)).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO media.object_location(id,profile_id,bucket,object_key,state,byte_size,sha256,mime_type)VALUES(?,?,?,?,'accepted',?,?,?)`, location, profile, ref.Bucket, ref.Key, len(body), digest, mime).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO media.media_asset(id,project_id,kind,origin,status,object_key,object_location_id,file_name,mime_type,codec,byte_size,sha256,moderation_status)VALUES(?,?,?,'upload','ready',?,?,?,?,?,?,?,'passed')`, id, grant.Scope.ProjectID, kind, ref.Key, location, "reference."+extension, mime, extension, len(body), digest).Error; err != nil {
		t.Fatal(err)
	}
	return id, ref
}
