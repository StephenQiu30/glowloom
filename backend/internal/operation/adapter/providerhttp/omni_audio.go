package providerhttp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

var audioMention = regexp.MustCompile(`@音频([0-9]+)`)

func (c *Client) submitOmniAudio(ctx context.Context, in application.GenerationRequest, key string) (application.GenerationResult, error) {
	invalid := application.GenerationResult{Outcome: "not_submitted", ErrorCode: "invalid_provider_request"}
	if in.Config.Validate() != nil || !validKey(key) || in.ModelID != "seed-audio-1.0" || in.Mode != "omni_audio" || in.Capability != "audio.generate" || in.OutputCount != 1 || in.EndpointTag != "" || strings.TrimSpace(in.Prompt) == "" || utf8.RuneCountInString(in.Prompt) > 3000 || len(in.References) > 3 {
		return invalid, nil
	}
	refs := []map[string]string{}
	kind := ""
	for _, r := range in.References {
		u, err := url.Parse(r.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || len(r.URL) > 8192 || kind != "" && kind != r.MediaKind {
			return invalid, nil
		}
		kind = r.MediaKind
		switch {
		case kind == "audio" && r.Role == "reference_audio":
			refs = append(refs, map[string]string{"audio_url": r.URL})
		case kind == "image" && r.Role == "reference_image" && len(in.References) == 1:
			refs = append(refs, map[string]string{"image_url": r.URL})
		default:
			return invalid, nil
		}
	}
	for _, m := range audioMention.FindAllStringSubmatch(in.Prompt, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil || kind != "audio" || n < 1 || n > len(refs) {
			return invalid, nil
		}
	}
	audioConfig := map[string]any{"format": "mp3", "sample_rate": 24000}
	for field, value := range in.Params {
		switch field {
		case "speech_rate", "loudness_rate", "pitch_rate":
			n, ok := integerParam(value)
			lo, hi := -50, 100
			if field == "pitch_rate" {
				lo, hi = -12, 12
			}
			if !ok || n < int64(lo) || n > int64(hi) {
				return invalid, nil
			}
			audioConfig[field] = n
		case "enable_subtitle":
			b, ok := value.(bool)
			if !ok {
				return invalid, nil
			}
			audioConfig[field] = b
		default:
			return invalid, nil
		}
	}
	payload := map[string]any{"model": in.ModelID, "text_prompt": in.Prompt, "audio_config": audioConfig}
	if len(refs) > 0 {
		payload["references"] = refs
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return invalid, nil
	}
	timeout, cancel := context.WithTimeout(ctx, time.Duration(in.Config.TimeoutSeconds)*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(timeout, http.MethodPost, strings.TrimRight(in.Config.BaseURL, "/")+"/tts/create", bytes.NewReader(body))
	if err != nil {
		return invalid, nil
	}
	id := in.ProviderRequestID
	if id == "" {
		id = uuid.NewString()
	}
	if _, err := uuid.Parse(id); err != nil {
		return invalid, nil
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", key)
	req.Header.Set("X-Api-Request-Id", id)
	unknown := application.GenerationResult{Outcome: "unknown", ErrorCode: "response_unconfirmed"}
	resp, err := c.client.Do(req)
	if err != nil {
		return unknown, nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return submitHTTPFailure(resp.StatusCode), nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (48<<20)+1))
	if err != nil || len(data) > 48<<20 {
		return unknown, nil
	}
	var result struct {
		Code             *int                       `json:"code"`
		Audio            string                     `json:"audio"`
		Duration         float64                    `json:"duration"`
		OriginalDuration float64                    `json:"original_duration"`
		Subtitle         *application.AudioSubtitle `json:"subtitle"`
	}
	if json.Unmarshal(data, &result) != nil {
		return unknown, nil
	}
	if result.Code != nil && *result.Code != 0 {
		unknown.ProviderTerminalCode = *result.Code
		return unknown, nil
	}
	if result.Duration <= 0 || result.Duration > 240 || result.OriginalDuration <= 0 || result.OriginalDuration > 120 || !result.Subtitle.Valid(int64(math.Ceil(result.Duration*1000))) {
		return unknown, nil
	}
	audio, err := base64.StdEncoding.DecodeString(result.Audio)
	if err != nil || len(audio) == 0 || len(audio) > 32<<20 || http.DetectContentType(audio) != "audio/mpeg" {
		return unknown, nil
	}
	usage, _ := json.Marshal(map[string]float64{"original_duration": result.OriginalDuration, "duration": result.Duration})
	return application.GenerationResult{Outcome: "completed", State: "succeeded", Audio: []application.GeneratedAudio{{Bytes: audio, MediaType: "audio/mpeg"}}, Usage: usage, Subtitle: result.Subtitle}, nil
}
