package media_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

func TestNormalizedUploadHTTPRequiresReviewAndReturnsCanonicalSafeAsset(t *testing.T) {
	data := uploadWebMFixture(t, "libvpx-vp9", "0.6")
	for _, confirmed := range []bool{false, true} {
		repo := &uploadRepoFake{}
		router := uploadHTTPRouter(repo, mediaflow.FFUploadProber{})
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", "白膜.webm")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatal(err)
		}
		if confirmed {
			if err := writer.WriteField("local_review_confirmed", "true"); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequestWithContext(t.Context(), "POST", "/api/projects/"+uuid.NewString()+"/media/uploads", &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		request.Header.Set("Origin", "http://localhost:3000")
		request.Header.Set("Idempotency-Key", uuid.NewString())
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if !confirmed {
			if response.Code != 422 || repo.commits != 0 {
				t.Fatalf("unreviewed WebM publication status=%d commits=%d", response.Code, repo.commits)
			}
			continue
		}
		var result mediaapp.UploadResult
		if response.Code != 201 || json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Asset.FileName != "白膜.mp4" ||
			result.Asset.MIMEType != "video/mp4" || result.Asset.Kind != "video" || result.Asset.DurationMS == nil || *result.Asset.DurationMS != 600 ||
			bytes.Contains(response.Body.Bytes(), []byte("normalization")) || bytes.Contains(response.Body.Bytes(), []byte("object_key")) {
			t.Fatalf("canonical public receipt status=%d body=%s", response.Code, response.Body.String())
		}
	}
}
