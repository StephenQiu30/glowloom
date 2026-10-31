package providerhttp

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	catalogapp "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/domain"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// VolcengineMusicDriver implements the communication strategy for Volcengine Music
// (Imagination) using AWS IAM V4 HMAC-SHA256 signature protocol.
type VolcengineMusicDriver struct{}

func init() {
	RegisterDriver(&VolcengineMusicDriver{}, "volcengine_music")
}

func (d *VolcengineMusicDriver) Submit(ctx context.Context, c *Client, input application.GenerationRequest, key string) (application.GenerationResult, error) {
	return c.submitMusic(ctx, input, key)
}

func (d *VolcengineMusicDriver) Query(ctx context.Context, c *Client, config catalog.ProviderConfig, taskID, key string) (application.GenerationResult, error) {
	return c.queryMusic(ctx, config, taskID, key)
}

func (d *VolcengineMusicDriver) Probe(ctx context.Context, c *Client, config catalog.ProviderConfig, key string) ProbeResult {
	if config.Validate() != nil {
		return ProbeResult{Auth: "unknown", Adapter: "unsupported", Discovery: "unsupported", Candidates: []catalogapp.ModelCandidate{}}
	}
	return ProbeResult{Auth: "unknown", Adapter: "ok", Discovery: "unsupported", Candidates: []catalogapp.ModelCandidate{}}
}

func (d *VolcengineMusicDriver) ProbeModelDefaultParams(capability string) map[string]any {
	return nil
}

func (c *Client) musicRequest(ctx context.Context, config catalog.ProviderConfig, key, action string, payload any) ([]byte, int, error) {
	var credential struct {
		AK string `json:"access_key_id"`
		SK string `json:"secret_access_key"`
	}
	if config.Validate() != nil || json.Unmarshal([]byte(key), &credential) != nil || !validKey(credential.AK) || !validKey(credential.SK) {
		return nil, 0, application.ErrInvalidProviderCall
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, 0, err
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(config.TimeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, config.BaseURL+"/?Action="+action+"&Version=2024-08-12", bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	signMusicRequest(req, body, credential.AK, credential.SK, time.Now().UTC())
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
	if len(data) > 1<<20 {
		return nil, resp.StatusCode, application.ErrInvalidProviderCall
	}
	return data, resp.StatusCode, err
}

// signMusicRequest implements the official imagination signing example using
// the exact transmitted JSON bytes. Secrets never enter query strings or logs.
func signMusicRequest(req *http.Request, body []byte, ak, sk string, now time.Time) {
	stamp := now.UTC().Format("20060102T150405Z")
	date := stamp[:8]
	hash := func(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
	mac := func(key []byte, value string) []byte {
		m := hmac.New(sha256.New, key)
		_, _ = m.Write([]byte(value))
		return m.Sum(nil)
	}
	bodyHash := hash(body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Date", stamp)
	req.Header.Set("X-Content-Sha256", bodyHash)
	signed := "content-type;host;x-content-sha256;x-date"
	headers := "content-type:application/json\nhost:" + req.URL.Host + "\nx-content-sha256:" + bodyHash + "\nx-date:" + stamp + "\n"
	canonical := strings.Join([]string{req.Method, "/", req.URL.Query().Encode(), headers, signed, bodyHash}, "\n")
	scope := date + "/cn-beijing/imagination/request"
	toSign := "HMAC-SHA256\n" + stamp + "\n" + scope + "\n" + hash([]byte(canonical))
	derived := mac(mac(mac(mac([]byte(sk), date), "cn-beijing"), "imagination"), "request")
	req.Header.Set("Authorization", "HMAC-SHA256 Credential="+ak+"/"+scope+", SignedHeaders="+signed+", Signature="+hex.EncodeToString(mac(derived, toSign)))
}

func (c *Client) submitMusic(ctx context.Context, input application.GenerationRequest, key string) (application.GenerationResult, error) {
	invalid := application.GenerationResult{Outcome: "not_submitted", ErrorCode: "invalid_provider_request"}
	var credential struct {
		AK string `json:"access_key_id"`
		SK string `json:"secret_access_key"`
	}
	if json.Unmarshal([]byte(key), &credential) != nil || !validKey(credential.AK) || !validKey(credential.SK) {
		return invalid, nil
	}
	if input.Config.Validate() != nil || input.ModelID != "v5.0" || input.Capability != "audio.generate" || input.OutputCount != 1 || input.EndpointTag != "" || len(input.References) != 0 || strings.TrimSpace(input.Prompt) == "" || utf8.RuneCountInString(input.Prompt) > 2000 {
		return invalid, nil
	}
	ms, ok := integerParam(input.Params["duration_ms"])
	if !ok || ms%1000 != 0 || ms < 30000 || ms > 240000 {
		return invalid, nil
	}
	payload := map[string]any{"Duration": ms / 1000}
	action := "GenSongForTime"
	switch input.Mode {
	case "music_song":
		length := utf8.RuneCountInString(input.Prompt)
		if length < 5 || strings.ContainsFunc(input.Prompt, func(r rune) bool { return unicode.Is(unicode.Han, r) }) && length > 700 {
			return invalid, nil
		}
		field := "Prompt"
		if mode, exists := input.Params["music_input"]; exists {
			if mode == "lyrics" {
				field = "Lyrics"
			} else if mode != "description" {
				return invalid, nil
			}
		}
		payload[field] = input.Prompt
		payload["ModelVersion"] = "v5.0"
		payload["VodFormat"] = "mp3"
		payload["SkipCopyCheck"] = false
	case "music_instrumental":
		if ms > 120000 {
			return invalid, nil
		}
		action = "GenBGMForTime"
		payload["Text"] = input.Prompt
		payload["Version"] = "v5.0"
		payload["EnableInputRewrite"] = false
		// Segments have higher priority than durations written inside Text.
		// A single section freezes the requested duration for cost admission.
		payload["Segments"] = []map[string]any{{"Name": "inst", "Duration": ms / 1000}}
	default:
		return invalid, nil
	}
	for field := range input.Params {
		if field != "duration_ms" && (input.Mode != "music_song" || field != "music_input") {
			return invalid, nil
		}
	}
	data, status, err := c.musicRequest(ctx, input.Config, key, action, payload)
	if err != nil {
		return application.GenerationResult{Outcome: "unknown", ErrorCode: "transport_unknown"}, nil
	}
	if status < 200 || status >= 300 {
		return submitHTTPFailure(status), nil
	}
	var response musicResponse
	if json.Unmarshal(data, &response) != nil || response.Code == nil || hasProviderError(response.Metadata.Error) {
		return application.GenerationResult{Outcome: "unknown", ErrorCode: "invalid_response"}, nil
	}
	if *response.Code != 0 {
		return musicError(*response.Code, true), nil
	}
	if response.Result == nil || !validTaskID(response.Result.TaskID) {
		return application.GenerationResult{Outcome: "unknown", ErrorCode: "invalid_response"}, nil
	}
	return application.GenerationResult{Outcome: "accepted", State: "pending", TaskID: response.Result.TaskID}, nil
}

type musicResponse struct {
	Code     *int `json:"Code"`
	Metadata struct {
		Error json.RawMessage `json:"Error"`
	} `json:"ResponseMetadata"`
	Result *struct {
		TaskID     string `json:"TaskID"`
		Status     *int   `json:"Status"`
		SongDetail *struct {
			AudioURL string      `json:"AudioUrl"`
			Duration json.Number `json:"Duration"`
		} `json:"SongDetail"`
	} `json:"Result"`
}

func (c *Client) queryMusic(ctx context.Context, config catalog.ProviderConfig, taskID, key string) (application.GenerationResult, error) {
	if config.Validate() != nil || !validTaskID(taskID) {
		return unknownResult(), nil
	}
	data, status, err := c.musicRequest(ctx, config, key, "QuerySong", map[string]string{"TaskID": taskID})
	if err != nil || status != 200 {
		return unknownResult(), nil
	}
	var response musicResponse
	if json.Unmarshal(data, &response) != nil || response.Code == nil || hasProviderError(response.Metadata.Error) {
		return unknownResult(), nil
	}
	if *response.Code != 0 {
		return musicError(*response.Code, false), nil
	}
	if response.Result == nil || response.Result.Status == nil || response.Result.TaskID != "" && response.Result.TaskID != taskID {
		return unknownResult(), nil
	}
	result := application.GenerationResult{TaskID: taskID}
	switch *response.Result.Status {
	case 0:
		result.State = "pending"
	case 1:
		result.State = "running"
	case 2:
		detail := response.Result.SongDetail
		if detail == nil || !catalog.ValidCreationResultURL(config.AdapterKey, detail.AudioURL) {
			return unknownResult(), nil
		}
		seconds, err := detail.Duration.Float64()
		if err != nil || seconds <= 0 || seconds > 240 {
			return unknownResult(), nil
		}
		result.State = "succeeded"
		result.URLs = []string{detail.AudioURL}
		result.Usage, _ = json.Marshal(map[string]any{"duration_seconds": detail.Duration})
	case 3:
		result.State = "failed"
		result.ErrorCode = "generation_failed"
	default:
		return unknownResult(), nil
	}
	return result, nil
}

func musicError(code int, submitting bool) application.GenerationResult {
	result := application.GenerationResult{State: "unknown", ProviderTerminalCode: code, ErrorCode: "provider_error"}
	if submitting {
		result.Outcome = "unknown"
		switch code {
		case 100010, 100011, 200020, 200021, 200022, 200023, 200024, 200026, 200027, 200028, 300061, 300063, 300065, 300066, 400040, 50000001:
			result.Outcome = "rejected"
		}
	}
	return result
}

func hasProviderError(raw json.RawMessage) bool { return len(raw) > 0 && string(raw) != "null" }
