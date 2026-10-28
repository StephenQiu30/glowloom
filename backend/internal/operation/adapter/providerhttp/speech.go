package providerhttp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// submitSpeech is deliberately separate from Ark's Bearer/JSON protocol. A speech
// Key is sent only to the pinned speech origin, never to a discovered endpoint.
func (c *Client) submitSpeech(ctx context.Context, input application.GenerationRequest, key string) (application.GenerationResult, error) {
	rejected := application.GenerationResult{Outcome: "not_submitted", ErrorCode: "invalid_provider_request"}
	if input.Config.Validate() != nil || !validKey(key) || input.ModelID != "seed-tts-2.0" || input.Capability != "audio.generate" || input.Mode != "text2audio" || input.OutputCount != 1 || len(input.References) != 0 || input.EndpointTag != "" || strings.TrimSpace(input.Prompt) == "" || len(input.Prompt) > 16384 {
		return rejected, nil
	}
	requestID := input.ProviderRequestID
	if requestID == "" {
		requestID = uuid.NewString()
	} else if id, err := uuid.Parse(requestID); err != nil || id == uuid.Nil || id.String() != requestID {
		return rejected, nil
	}
	speaker := "zh_female_vv_uranus_bigtts"
	audio := map[string]any{"format": "mp3", "sample_rate": 24000}
	for field, value := range input.Params {
		switch field {
		case "speaker":
			var ok bool
			speaker, ok = value.(string)
			if !ok || len(speaker) == 0 || len(speaker) > 128 {
				return rejected, nil
			}
			for _, r := range speaker {
				allowed := r == '_' || r == '-' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
				if !allowed {
					return rejected, nil
				}
			}
		case "speech_rate", "loudness_rate":
			raw, err := json.Marshal(value)
			var rate int
			if err != nil || json.Unmarshal(raw, &rate) != nil || rate < -50 || rate > 100 {
				return rejected, nil
			}
			audio[field] = rate
		default:
			return rejected, nil
		}
	}
	payload, err := json.Marshal(map[string]any{"req_params": map[string]any{"text": input.Prompt, "speaker": speaker, "audio_params": audio}})
	if err != nil {
		return rejected, nil
	}
	timeout, cancel := context.WithTimeout(ctx, time.Duration(input.Config.TimeoutSeconds)*time.Second)
	defer cancel()
	endpoint := input.Config.BaseURL + "/tts/unidirectional"
	if input.StrictSpeechTerminal {
		endpoint += "/sse"
	}
	request, err := http.NewRequestWithContext(timeout, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return rejected, nil
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Api-Key", key)
	request.Header.Set("X-Api-Resource-Id", input.ModelID)
	request.Header.Set("X-Api-Request-Id", requestID)
	request.Header.Set("X-Control-Require-Usage-Tokens-Return", "*")
	response, err := c.client.Do(request)
	if err != nil {
		return unknownResult(), nil
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		outcome := "unknown"
		if response.StatusCode == 400 || response.StatusCode == 401 || response.StatusCode == 402 || response.StatusCode == 403 || response.StatusCode == 404 || response.StatusCode == 422 || response.StatusCode == 429 {
			outcome = "rejected"
		}
		return application.GenerationResult{Outcome: outcome, ErrorCode: safeHTTPError(response.StatusCode)}, nil
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (64<<20)+1))
	if err != nil || len(data) > 64<<20 {
		return unknownResult(), nil
	}
	if input.StrictSpeechTerminal {
		if !strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
			return unknownResult(), nil
		}
		var err error
		data, err = speechSSEFrames(data)
		if err != nil {
			return unknownResult(), nil
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	output := []byte{}
	var usage json.RawMessage
	finished := false
	for {
		var chunk struct {
			Code  *int            `json:"code"`
			Data  string          `json:"data"`
			Usage json.RawMessage `json:"usage"`
		}
		err := decoder.Decode(&chunk)
		if err == io.EOF {
			break
		}
		if err != nil || chunk.Code == nil || finished {
			return unknownResult(), nil
		}
		if *chunk.Code == 20000000 {
			if chunk.Data != "" {
				return unknownResult(), nil
			}
			if len(chunk.Usage) > 0 {
				usage = safeSpeechUsage(chunk.Usage)
			}
			finished = true
			continue
		}
		if *chunk.Code != 0 {
			if len(output) > 0 {
				return unknownResult(), nil
			}
			return application.GenerationResult{Outcome: "rejected", ErrorCode: "invalid_parameters"}, nil
		}
		if chunk.Data != "" {
			part, err := base64.StdEncoding.Strict().DecodeString(chunk.Data)
			if err != nil || len(output)+len(part) > 32<<20 {
				return unknownResult(), nil
			}
			output = append(output, part...)
		}
		if len(chunk.Usage) > 0 {
			usage = safeSpeechUsage(chunk.Usage)
		}
	}
	// Current HTTP documentation completes with code=0 and a clean HTTP EOF.
	// The legacy 20000000 terminal marker is accepted but is not mandatory.
	if input.StrictSpeechTerminal && !finished {
		return unknownResult(), nil
	}
	if len(output) == 0 || http.DetectContentType(output) != "audio/mpeg" {
		return unknownResult(), nil
	}
	logID := response.Header.Get("X-Tt-Logid")
	if len(logID) > 256 || strings.ContainsAny(logID, "\r\n\x00") {
		logID = ""
	}
	terminalCode := 0
	if finished {
		terminalCode = 20000000
	}
	return application.GenerationResult{Outcome: "completed", State: "succeeded", ProviderTerminalCode: terminalCode, ProviderLogID: logID, Audio: []application.GeneratedAudio{{Bytes: output, MediaType: "audio/mpeg"}}, Usage: usage}, nil
}

func safeSpeechUsage(raw json.RawMessage) json.RawMessage {
	var value struct {
		TextWords *int64 `json:"text_words"`
	}
	if json.Unmarshal(raw, &value) != nil || value.TextWords == nil || *value.TextWords < 0 {
		return nil
	}
	result, _ := json.Marshal(map[string]int64{"text_words": *value.TextWords})
	return result
}

// speechSSEFrames retains only complete SSE data events. A partial last event,
// malformed JSON or a bare EOF can never prove the paid synthesis completed.
func speechSSEFrames(data []byte) ([]byte, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 64<<20)
	var output bytes.Buffer
	var event []string
	flush := func() error {
		if len(event) == 0 {
			return nil
		}
		raw := strings.Join(event, "\n")
		if !json.Valid([]byte(raw)) {
			return errors.New("invalid speech event")
		}
		output.WriteString(raw)
		output.WriteByte('\n')
		event = nil
		return nil
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			return nil, errors.New("invalid speech event")
		}
		value = strings.TrimPrefix(value, " ")
		switch field {
		case "data":
			event = append(event, value)
		case "event", "id", "retry":
		default:
			return nil, errors.New("invalid speech event")
		}
	}
	if scanner.Err() != nil || len(event) > 0 {
		return nil, errors.New("incomplete speech event")
	}
	return output.Bytes(), nil
}
