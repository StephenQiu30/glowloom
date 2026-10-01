// Package whisper consumes the pinned native whisper.cpp inference protocol.
package whisper

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"unicode/utf8"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
	"github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

// Client uploads worker-owned PCM files to an explicitly configured loopback service.
// It never forwards media to redirects or an environment HTTP proxy.
type Client struct {
	endpoint string
	http     *http.Client
}

// NewClient validates the local endpoint and injects the actual HTTP transport.
func NewClient(endpoint string, client *http.Client) (*Client, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.Port() == "" {
		return nil, application.ErrInvalidTranscription
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return nil, application.ErrInvalidTranscription
	}
	if client == nil {
		return nil, application.ErrUnavailable
	}
	own := *client
	own.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	if own.Timeout == 0 {
		own.Timeout = application.TranscriptionInferenceTimeout
	}
	if transport, ok := own.Transport.(*http.Transport); ok {
		private := transport.Clone()
		private.Proxy = nil
		own.Transport = private
	}
	if own.Transport == nil {
		private := http.DefaultTransport.(*http.Transport).Clone()
		private.Proxy = nil
		own.Transport = private
	}
	return &Client{endpoint: strings.TrimRight(u.String(), "/") + "/inference", http: &own}, nil
}

// Transcribe sends one bounded WAV and validates native timings against its full duration.
// A transport error after dispatch retains uncertainty about remote completion.
func (c *Client) Transcribe(ctx context.Context, wav *mediaapp.Downloaded, language string, durationMS int64) (domain.Transcript, error) {
	if c == nil || c.http == nil {
		return domain.Transcript{}, application.ErrUnavailable
	}
	if wav == nil || wav.File == nil || wav.MIMEType != "audio/wav" || wav.Size < 44 || wav.Size > 512<<20 || durationMS < 1 || durationMS > 86400000 || !domain.ValidTranscriptionLanguage(language) {
		return domain.Transcript{}, application.ErrInvalidTranscription
	}
	if err := ctx.Err(); err != nil {
		return domain.Transcript{}, err
	}
	// A temporary multipart body bounds memory independently from the WAV budget.
	// Its private filename is owned and removed by this call.
	body, err := os.CreateTemp("", "lanverse-transcription-multipart-*")
	if err != nil {
		return domain.Transcript{}, fmt.Errorf("create inference upload: %w", err)
	}
	defer func() { _ = body.Close(); _ = os.Remove(body.Name()) }()
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "audio16k.wav")
	if err != nil {
		return domain.Transcript{}, err
	}
	if _, err = wav.File.Seek(0, io.SeekStart); err != nil {
		return domain.Transcript{}, err
	}
	n, err := io.Copy(part, io.LimitReader(wav.File, wav.Size+1))
	if err != nil {
		return domain.Transcript{}, fmt.Errorf("prepare inference upload: %w", err)
	}
	if n != wav.Size {
		return domain.Transcript{}, application.ErrInvalidTranscription
	}
	for _, field := range [][2]string{{"response_format", "verbose_json"}, {"language", language}, {"translate", "false"}, {"no_timestamps", "false"}, {"token_timestamps", "false"}} {
		if err = writer.WriteField(field[0], field[1]); err != nil {
			return domain.Transcript{}, err
		}
	}
	if err = writer.Close(); err != nil {
		return domain.Transcript{}, err
	}
	size, err := body.Seek(0, io.SeekEnd)
	if err != nil {
		return domain.Transcript{}, err
	}
	if _, err = body.Seek(0, io.SeekStart); err != nil {
		return domain.Transcript{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, body)
	if err != nil {
		return domain.Transcript{}, err
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := c.http.Do(req)
	if err != nil {
		var network *net.OpError
		if errors.As(err, &network) && network.Op == "dial" {
			return domain.Transcript{}, fmt.Errorf("%w: native inference connection failed", application.ErrUnavailable)
		}
		return domain.Transcript{}, fmt.Errorf("%w: %w", application.ErrInferenceUncertain, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		// Raw server responses can contain private recognized speech or local paths.
		return domain.Transcript{}, fmt.Errorf("%w: native inference HTTP %d", application.ErrInvalidTranscription, response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (8<<20)+1))
	if err != nil {
		return domain.Transcript{}, fmt.Errorf("%w: read native result: %w", application.ErrInferenceUncertain, err)
	}
	if len(raw) > 8<<20 {
		return domain.Transcript{}, domain.ErrInvalidTranscript
	}
	return decode(raw, durationMS)
}

type nativeSegment struct {
	Start *float64 `json:"start"`
	End   *float64 `json:"end"`
	Text  string   `json:"text"`
}
type nativeResult struct {
	Language string          `json:"language"`
	Duration *float64        `json:"duration"`
	Segments []nativeSegment `json:"segments"`
}

func decode(raw []byte, durationMS int64) (domain.Transcript, error) {
	if !utf8.Valid(raw) {
		return domain.Transcript{}, domain.ErrInvalidTranscript
	}
	var result nativeResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return domain.Transcript{}, fmt.Errorf("%w: decode native result", domain.ErrInvalidTranscript)
	}
	if result.Duration == nil || !finite(*result.Duration) || *result.Duration <= 0 || *result.Duration > 86400 || math.Abs(*result.Duration*1000-float64(durationMS)) > 25 || len(result.Segments) > 10000 {
		return domain.Transcript{}, domain.ErrInvalidTranscript
	}
	transcript := domain.Transcript{Version: 1, Language: result.Language, DurationMS: int64(math.Round(*result.Duration * 1000))}
	previousEnd := 0.0
	for _, cue := range result.Segments {
		if cue.Start == nil || cue.End == nil || !finite(*cue.Start) || !finite(*cue.End) || *cue.Start < previousEnd || *cue.End <= *cue.Start || *cue.End > *result.Duration {
			return domain.Transcript{}, domain.ErrInvalidTranscript
		}
		previousEnd = *cue.End
		text := strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(cue.Text, "\r\n", "\n"), "\r", "\n"))
		if text == "" {
			continue
		}
		transcript.Segments = append(transcript.Segments, domain.SubtitleSegment{StartMS: int64(math.Round(*cue.Start * 1000)), EndMS: int64(math.Round(*cue.End * 1000)), Text: text})
	}
	if len(transcript.Segments) == 0 {
		return domain.Transcript{}, application.ErrNoSpeech
	}
	if err := transcript.Validate(); err != nil {
		return domain.Transcript{}, err
	}
	return transcript, nil
}
func finite(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) }
