package aigateway_test

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app/aigateway"
	gatewayhttp "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/http"
	pg "github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/adapter/postgres"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

func testProviderKeyMAC(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	_, err := rand.Read(key)
	require.NoError(t, err)
	return key
}

func TestProviderKeyOnboardingSharesOnlyMatchingVerifiedScopes(t *testing.T) {
	db, _, live, actor := fixture(t)
	entrypointProject(t, db, actor)
	entrypointSession(t, db, actor)
	ctx := t.Context()
	var initialProofs int
	require.NoError(t, db.Raw("SELECT count(*) FROM ai.provider_key_verification").Row().Scan(&initialProofs))
	require.NoError(t, db.Exec("UPDATE ai.account SET available=0 WHERE scope_id=?", actor.ScopeID).Error)

	type upstreamEndpoint struct {
		base, host, path, query, body string
	}
	modelList := `{"object":"list","data":[]}`
	upstreams := map[string]upstreamEndpoint{
		"ark":        {"https://ark.cn-beijing.volces.com/api/v3", "ark.cn-beijing.volces.com", "/api/v3/models", "", modelList},
		"openai":     {"https://api.openai.com/v1", "api.openai.com", "/v1/models", "", modelList},
		"deepseek":   {"https://api.deepseek.com", "api.deepseek.com", "/models", "", modelList},
		"router":     {"https://openrouter.ai/api/v1", "openrouter.ai", "/api/v1/key", "", `{"data":{"label":"test"}}`},
		"vercel":     {"https://ai-gateway.vercel.sh/v1", "ai-gateway.vercel.sh", "/v1/credits", "", `{"balance":"0","total_used":"0.1"}`},
		"byteplus":   {"https://ark.ap-southeast.bytepluses.com/api/v3", "ark.ap-southeast.bytepluses.com", "/api/v3/contents/generations/tasks", "page_num=1&page_size=1", `{"total":0,"items":[]}`},
		"anthropic":  {"https://api.anthropic.com/v1", "api.anthropic.com", "/v1/models", "limit=1", `{"data":[],"has_more":false,"first_id":null,"last_id":null}`},
		"deepseekV1": {"https://api.deepseek.com/v1", "api.deepseek.com", "/v1/models", "", modelList},
	}
	byRequest := make(map[string]upstreamEndpoint)
	for _, upstream := range upstreams {
		byRequest[upstream.host+upstream.path] = upstream
	}
	var mu sync.Mutex
	calls := make(map[string]int)
	client := mediaClient(func(req *http.Request) (*http.Response, error) {
		upstream, exists := byRequest[req.Host+req.URL.Path]
		if !exists || req.Method != http.MethodGet || req.URL.RawQuery != upstream.query || req.Body != nil {
			t.Errorf("unexpected provider request %s %s%s", req.Method, req.Host, req.URL.Path)
			return mediaResponse(http.StatusBadRequest, "{}"), nil
		}
		key := strings.TrimPrefix(req.Header.Get("Authorization"), "Bearer ")
		if upstream.host == "api.anthropic.com" {
			key = req.Header.Get("X-Api-Key")
			if req.Header.Get("Anthropic-Version") != "2023-06-01" {
				t.Error("missing Anthropic-Version")
			}
		}
		mu.Lock()
		calls[upstream.base+"/"+key]++
		mu.Unlock()
		switch key {
		case "bad-key":
			return mediaResponse(http.StatusUnauthorized, `{"error":{"message":"private"}}`), nil
		case "management-key":
			return mediaResponse(http.StatusOK, `{"data":{"label":"private","is_management_key":true}}`), nil
		case "invalid-response":
			return mediaResponse(http.StatusOK, "{}"), nil
		case "shared-key", "rotated-key":
			return mediaResponse(http.StatusOK, upstream.body), nil
		default:
			t.Error("missing expected credential header")
			return mediaResponse(http.StatusUnauthorized, "{}"), nil
		}
	})
	engine := gin.New()
	api := engine.Group("/api")
	session := api.Group("", func(c *gin.Context) { c.Set("principal", actor) })
	gatewayhttp.New(db, aigateway.Authority, client, onboardingSealer{}, gatewayhttp.KeyVerification{
		Verify: client.VerifyProviderKey, MACKey: testProviderKeyMAC(t),
	}).Register(api, session)
	post := func(path string, raw []byte) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/ai/admin/"+path, bytes.NewReader(raw)))
		return w
	}
	prepare := func(base, protocol string) (uuid.UUID, *httptest.ResponseRecorder) {
		model := live
		model.ID, model.Name, model.Protocol = uuid.New(), "key-"+uuid.NewString(), protocol
		model.Tools, model.Stream = false, false
		raw, err := json.Marshal(model)
		require.NoError(t, err)
		stage := post("models", raw)
		require.Equal(t, http.StatusCreated, stage.Code, stage.Body.String())
		route := domain.Route{ID: uuid.New(), ModelID: model.ID, Supplier: uuid.New(), Group: uuid.NewString(), Protocol: protocol,
			BaseURL: base, UpstreamModel: "fictional-model", Purchase: model.Price}
		raw, err = json.Marshal(application.Onboarding{Route: route, Scope: actor.ScopeID, Concurrency: 1, RPM: 1})
		require.NoError(t, err)
		return route.ID, post("routes", raw)
	}
	prepareReadOnly := func(base, protocol string) uuid.UUID {
		id, w := prepare(base, protocol)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var review pg.OnboardingReview
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &review))
		require.True(t, review.Plan.ReadOnlyKey())
		require.Zero(t, review.Plan.Credits)
		require.Empty(t, review.Plan.Probes)
		return id
	}
	verify := func(id uuid.UUID, key string) *httptest.ResponseRecorder {
		return post("onboarding/"+id.String()+"/verify", []byte(`{"key":"`+key+`","credits":0,"purchase_micros":0}`))
	}

	openAI := upstreams["openai"].base
	first, second := prepareReadOnly(openAI, "chat"), prepareReadOnly(openAI, "chat")
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, 2)
	for _, id := range []uuid.UUID{first, second} {
		go func() {
			<-start
			results <- verify(id, "shared-key")
		}()
	}
	close(start)
	for range 2 {
		w := <-results
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.NotContains(t, w.Body.String(), "shared-key")
	}
	require.Equal(t, 1, calls[openAI+"/shared-key"])
	require.Equal(t, http.StatusOK, verify(first, "shared-key").Code)
	require.Equal(t, http.StatusConflict, verify(first, "rotated-key").Code)

	for name, upstream := range upstreams {
		if name == "openai" {
			continue
		}
		protocol := "chat"
		if name == "anthropic" {
			protocol = "messages"
		}
		w := verify(prepareReadOnly(upstream.base, protocol), "shared-key")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Equal(t, 1, calls[upstream.base+"/shared-key"])
	}
	w := verify(prepareReadOnly(openAI, "chat"), "rotated-key")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.Equal(t, 1, calls[openAI+"/rotated-key"])
	retry := prepareReadOnly(openAI, "chat")
	for range 2 {
		w = verify(retry, "bad-key")
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)
		require.Contains(t, w.Body.String(), "provider_key_rejected")
		require.NotContains(t, w.Body.String(), "private")
	}
	require.Equal(t, 2, calls[openAI+"/bad-key"])
	w = verify(retry, "invalid-response")
	require.NotEqual(t, http.StatusOK, w.Code)
	w = verify(prepareReadOnly(upstreams["router"].base, "chat"), "management-key")
	require.Equal(t, http.StatusUnprocessableEntity, w.Code)
	require.Contains(t, w.Body.String(), "provider_key_scope")

	_, w = prepare("https://compatible.example/v1", "chat")
	require.Equal(t, http.StatusUnprocessableEntity, w.Code, w.Body.String())
	require.Contains(t, w.Body.String(), "provider_key_check_unsupported")
	var rows int
	require.NoError(t, db.Raw(`SELECT count(*) FROM ai.provider_key_verification`).Row().Scan(&rows))
	require.Equal(t, initialProofs+len(upstreams)+1, rows)
	require.NoError(t, db.Raw(`SELECT count(*) FROM ai.request WHERE origin='onboarding' AND scope_id=?`, actor.ScopeID).Row().Scan(&rows))
	require.Zero(t, rows)
}

func TestVolcengineMediaKeyOnboardingDoesNotGenerateOrCharge(t *testing.T) {
	db, _, _, actor := fixture(t)
	entrypointProject(t, db, actor)
	entrypointSession(t, db, actor)
	ctx := t.Context()
	require.NoError(t, db.Exec("UPDATE ai.account SET available=0 WHERE scope_id=?", actor.ScopeID).Error)

	const (
		motionModel = "jimeng_dreamactor_m20_gen_video"
		avatarModel = "jimeng_realman_avatar_picture_omni_v15"
		speechPath  = "/api/v3/tts/bidirection"
		motionPath  = "/api/common/v3/get_result_v2"
		avatarPath  = "/api/common/v3/get_result"
	)
	reads := make(map[string]int)
	client := mediaClient(func(req *http.Request) (*http.Response, error) {
		reads[req.URL.Path]++
		switch req.URL.Path {
		case speechPath:
			require.Equal(t, "openspeech.bytedance.com", req.Host)
			require.Equal(t, http.MethodGet, req.Method)
			require.Nil(t, req.Body)
			require.Empty(t, req.Header.Get("Authorization"))
			require.Equal(t, "seed-tts-2.0", req.Header.Get("X-Api-Resource-Id"))
			require.Equal(t, "websocket", req.Header.Get("Upgrade"))
			require.NotEmpty(t, req.Header.Get("X-Api-Connect-Id"))
			switch req.Header.Get("X-Api-Key") {
			case "invalid-key":
				return mediaResponse(http.StatusUnauthorized, "{}"), nil
			case "unscoped-key":
				return mediaResponse(http.StatusForbidden, "{}"), nil
			case "shared-key", "invalid-upgrade":
				res := mediaResponse(http.StatusSwitchingProtocols, "")
				res.Header.Set("Upgrade", "websocket")
				res.Header.Set("Connection", "upgrade")
				// The protocol echo authenticates the switch to WebSocket.
				accept := sha1.Sum([]byte(req.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
				if req.Header.Get("X-Api-Key") == "shared-key" {
					res.Header.Set("Sec-WebSocket-Accept", base64.StdEncoding.EncodeToString(accept[:]))
				}
				return res, nil
			}
		case motionPath, avatarPath:
			require.Equal(t, "openapi.cv.volces.com", req.Host)
			require.Equal(t, http.MethodPost, req.Method)
			var input map[string]string
			require.NoError(t, json.NewDecoder(req.Body).Decode(&input))
			model := motionModel
			if req.URL.Path == avatarPath {
				model = avatarModel
			}
			require.Equal(t, map[string]string{"req_key": model, "task_id": "0"}, input)
			switch req.Header.Get("Authorization") {
			case "Bearer invalid-key":
				return mediaResponse(http.StatusUnauthorized, `{"code":50400,"data":null,"request_id":"test"}`), nil
			case "Bearer auth-business-error":
				return mediaResponse(http.StatusOK, `{"code":50400,"data":null,"request_id":"test"}`), nil
			case "Bearer rest-not-found":
				return mediaResponse(http.StatusNotFound, `{"code":10000,"data":{"status":"not_found"},"request_id":"test"}`), nil
			case "Bearer non-success-receipt":
				return mediaResponse(http.StatusOK, `{"code":50200,"data":{"status":"not_found"},"request_id":"test"}`), nil
			case "Bearer shared-key":
				return mediaResponse(http.StatusOK, `{"code":10000,"data":{"status":"not_found"},"request_id":"test"}`), nil
			}
		}
		t.Errorf("unexpected provider key request %s %s", req.Method, req.URL.Path)
		return mediaResponse(http.StatusBadRequest, "{}"), nil
	})

	engine := gin.New()
	api := engine.Group("/api")
	session := api.Group("", func(c *gin.Context) { c.Set("principal", actor) })
	gatewayhttp.New(db, aigateway.Authority, client, onboardingSealer{}, gatewayhttp.KeyVerification{
		Verify: client.VerifyProviderKey, MACKey: testProviderKeyMAC(t),
	}).Register(api, session)
	post := func(path string, input any) *httptest.ResponseRecorder {
		raw, err := json.Marshal(input)
		require.NoError(t, err)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequestWithContext(ctx, http.MethodPost, "/api/ai/admin/"+path, bytes.NewReader(raw)))
		return w
	}
	prepare := func(adapter, capability, mode, sku string) uuid.UUID {
		d := mediaDispatch(t, adapter, capability, mode, sku)
		model := d.Call.Model
		model.Name, model.PurchaseCap = sku+"-"+uuid.NewString(), 100000
		w := post("models", model)
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		route := d.Route
		route.ID, route.Supplier, route.Group = uuid.New(), uuid.New(), uuid.NewString()
		w = post("routes", application.Onboarding{Route: route, Scope: actor.ScopeID, Concurrency: 1, RPM: 60})
		require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
		var review pg.OnboardingReview
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &review))
		require.True(t, review.Plan.ReadOnlyKey())
		require.Zero(t, review.Plan.Credits)
		require.Empty(t, review.Plan.Probes)
		return route.ID
	}
	submit := func(id uuid.UUID, key string, trial bool) *httptest.ResponseRecorder {
		return post("onboarding/"+id.String()+"/verify", map[string]any{
			"key": key, "credits": 0, "purchase_micros": 0, "confirm_trial": trial,
		})
	}
	tts := prepare("volcengine_speech", "audio.generate", "text2audio", "seed-tts-2.0")
	audio := prepare("volcengine_speech", "audio.generate", "omni_audio", "seed-audio-1.0")
	motion := prepare("volcengine_motion", "video.generate", "motion", motionModel)
	avatar := prepare("volcengine_avatar", "video.generate", "avatar", avatarModel)
	require.Empty(t, reads)
	require.Equal(t, http.StatusBadRequest, submit(tts, "shared-key", true).Code)
	require.Empty(t, reads, "trial consent cannot switch the frozen key-only plan")
	require.Equal(t, http.StatusUnprocessableEntity, submit(tts, "invalid-key", false).Code)
	require.Equal(t, http.StatusUnprocessableEntity, submit(tts, "unscoped-key", false).Code)
	require.NotEqual(t, http.StatusOK, submit(tts, "invalid-upgrade", false).Code)
	require.Equal(t, http.StatusUnprocessableEntity, submit(motion, "invalid-key", false).Code)
	require.Equal(t, http.StatusUnprocessableEntity, submit(motion, "auth-business-error", false).Code)
	require.NotEqual(t, http.StatusOK, submit(motion, "rest-not-found", false).Code)
	require.NotEqual(t, http.StatusOK, submit(motion, "non-success-receipt", false).Code)
	for _, id := range []uuid.UUID{tts, audio, motion, avatar} {
		w := submit(id, "shared-key", false)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.NotContains(t, w.Body.String(), "shared-key")
		var active pg.OnboardingReview
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &active))
		require.Equal(t, "active", active.State)
		require.True(t, active.Available)
	}
	require.Equal(t, map[string]int{speechPath: 4, motionPath: 5, avatarPath: 1}, reads)

	var rows int
	require.NoError(t, db.Raw(`SELECT count(*) FROM ai.request WHERE origin='onboarding' AND scope_id=?`, actor.ScopeID).Row().Scan(&rows))
	require.Zero(t, rows)
	require.NoError(t, db.Raw(`SELECT count(*) FROM ai.budget WHERE scope_id=?`, actor.ScopeID).Row().Scan(&rows))
	require.Zero(t, rows)
}
