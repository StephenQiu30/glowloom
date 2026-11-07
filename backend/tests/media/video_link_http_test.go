package media_test

import (
	"context"
	"encoding/json"
	nethttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mediahttp "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

type videoLinkImportFixture func(context.Context, identity.Principal, uuid.UUID, uuid.UUID, application.VideoLinkRequest) (application.PersonalUploadResult, error)

func (f videoLinkImportFixture) Import(ctx context.Context, actor identity.Principal, key, request uuid.UUID, in application.VideoLinkRequest) (application.PersonalUploadResult, error) {
	return f(ctx, actor, key, request, in)
}
func videoLinkRouter(service mediahttp.VideoLinkImporter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("request_id", uuid.NewString())
		c.Set("principal", identity.Principal{ID: uuid.New(), OrgID: uuid.New()})
		c.Next()
	})
	mediahttp.NewVideoLinkHandler(service).Register(router.Group("/api"))
	return router
}
func videoLinkRequest(body string, key bool) *nethttp.Request {
	req := httptest.NewRequestWithContext(context.Background(), "POST", "/api/media/library/video-links", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if key {
		req.Header.Set("Idempotency-Key", uuid.NewString())
	}
	return req
}

func TestVideoLinkHTTPReturnsRealReceiptAndExplicitFailures(t *testing.T) {
	id := uuid.New()
	calls := 0
	router := videoLinkRouter(videoLinkImportFixture(func(_ context.Context, actor identity.Principal, key, request uuid.UUID, in application.VideoLinkRequest) (application.PersonalUploadResult, error) {
		calls++
		if actor.ID == uuid.Nil || key == uuid.Nil || request == uuid.Nil || !in.LocalReviewConfirmed {
			t.Fatal("identity or review missing")
		}
		if strings.Contains(in.URL, "unavailable") {
			return application.PersonalUploadResult{}, application.ErrVideoLinkUnresolvable
		}
		return application.PersonalUploadResult{Asset: application.LibraryAssetSummary{ID: id, Kind: "video"}}, nil
	}))
	for _, tc := range []struct {
		body   string
		key    bool
		status int
		code   string
	}{{`{"url":"https://v.douyin.com/test/","local_review_confirmed":true}`, true, 201, ""}, {`{"url":"https://v.douyin.com/unavailable/","local_review_confirmed":true}`, true, 422, "video_link_unresolvable"}, {`{"url":"https://v.douyin.com/test/","local_review_confirmed":true}`, false, 422, "invalid_idempotency_key"}, {`{"url":"https://v.douyin.com/test/","local_review_confirmed":false}`, true, 422, "invalid_video_link"}, {`{"url":"https://v.douyin.com/test/","local_review_confirmed":true,"asset_id":"invented"}`, true, 422, "invalid_request"}} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, videoLinkRequest(tc.body, tc.key))
		if response.Code != tc.status {
			t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
		}
		if tc.code != "" && !strings.Contains(response.Body.String(), tc.code) {
			t.Fatal(response.Body.String())
		}
		if tc.status == 201 {
			var result application.PersonalUploadResult
			if json.Unmarshal(response.Body.Bytes(), &result) != nil || result.Asset.ID != id || result.Asset.Kind != "video" {
				t.Fatal("missing actual asset receipt", response.Body.String())
			}
		}
	}
	if calls != 2 {
		t.Fatal("invalid body reached importer", calls)
	}
}

func TestVideoLinkHTTPBoundsConcurrentEgress(t *testing.T) {
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan struct{}, 2)
	router := videoLinkRouter(videoLinkImportFixture(func(ctx context.Context, _ identity.Principal, _, _ uuid.UUID, _ application.VideoLinkRequest) (application.PersonalUploadResult, error) {
		started <- struct{}{}
		select {
		case <-release:
			return application.PersonalUploadResult{Asset: application.LibraryAssetSummary{ID: uuid.New(), Kind: "video"}}, nil
		case <-ctx.Done():
			return application.PersonalUploadResult{}, ctx.Err()
		}
	}))
	defer func() {
		close(release)
		for range 2 {
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Error("request did not finish after release")
			}
		}
	}()
	for range 2 {
		go func() {
			defer func() { done <- struct{}{} }()
			router.ServeHTTP(httptest.NewRecorder(), videoLinkRequest(`{"url":"https://v.douyin.com/test/","local_review_confirmed":true}`, true))
		}()
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			t.Fatal("first two requests did not start")
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, videoLinkRequest(`{"url":"https://v.douyin.com/test/","local_review_confirmed":true}`, true))
	if response.Code != 429 {
		t.Fatal("unbounded egress", response.Code)
	}
}
