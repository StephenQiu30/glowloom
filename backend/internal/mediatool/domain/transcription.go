package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// ErrInvalidTranscript rejects malformed or invented subtitle timing facts.
var ErrInvalidTranscript = errors.New("invalid transcript")

// SubtitleSegment preserves one recognized cue at exact integer milliseconds.
type SubtitleSegment struct {
	StartMS int64  `json:"start_ms"`
	EndMS   int64  `json:"end_ms"`
	Text    string `json:"text"`
}

// Transcript is a draft requiring user review; it does not assert moderation or accuracy.
type Transcript struct {
	Version    int               `json:"version"`
	Language   string            `json:"language"`
	DurationMS int64             `json:"duration_ms"`
	Segments   []SubtitleSegment `json:"segments"`
}

// Validate bounds untrusted recognizer results before persistence or SRT output.
func (t Transcript) Validate() error {
	if t.Version != 1 || t.DurationMS < 1 || t.DurationMS > 86400000 || len(t.Language) < 1 || len(t.Language) > 64 || !utf8.ValidString(t.Language) || len(t.Segments) < 1 || len(t.Segments) > 10000 {
		return ErrInvalidTranscript
	}
	var previous int64
	for _, cue := range t.Segments {
		if cue.StartMS < previous || cue.EndMS <= cue.StartMS || cue.EndMS > t.DurationMS || len(cue.Text) > 32768 || strings.TrimSpace(cue.Text) == "" || !utf8.ValidString(cue.Text) {
			return ErrInvalidTranscript
		}
		for _, r := range cue.Text {
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				return ErrInvalidTranscript
			}
		}
		previous = cue.EndMS
	}
	raw, err := json.Marshal(t)
	if err != nil || len(raw) > 8<<20 {
		return ErrInvalidTranscript
	}
	return nil
}

// SRT renders valid cue facts without changing timing, order or recognized words.
func (t Transcript) SRT() (string, error) {
	if err := t.Validate(); err != nil {
		return "", err
	}
	var out strings.Builder
	for i, cue := range t.Segments {
		fmt.Fprintf(&out, "%d\n%02d:%02d:%02d,%03d --> %02d:%02d:%02d,%03d\n%s\n\n", i+1, cue.StartMS/3600000, cue.StartMS/60000%60, cue.StartMS/1000%60, cue.StartMS%1000, cue.EndMS/3600000, cue.EndMS/60000%60, cue.EndMS/1000%60, cue.EndMS%1000, cue.Text)
	}
	return out.String(), nil
}

// TranscriptionSource identifies a saved formal media node, never a local path.
type TranscriptionSource struct {
	CanvasID uuid.UUID `json:"canvas_id"`
	NodeID   uuid.UUID `json:"node_id"`
	Revision int64     `json:"revision"`
}

// ValidTranscriptionLanguage accepts native codes from fixed whisper.cpp 927cfce.
// The configured model can further reject a language it cannot recognize.
func ValidTranscriptionLanguage(language string) bool {
	return slices.Contains(strings.Fields("auto en zh de es ru ko fr ja pt tr pl ca nl ar sv it id hi fi vi he uk el ms cs ro da hu ta no th ur hr bg lt la mi ml cy sk te fa lv bn sr az sl kn et mk br eu is hy ne mn bs kk sq sw gl mr pa si km sn yo so af oc ka be tg sd gu am yi lo uz fo ht ps tk nn mt sa lb my bo tl mg as tt haw ln ha ba jw su yue"), language)
}

// TranscriptionStatus is local speech inference, independent of export review.
type TranscriptionStatus string

// Transcription states preserve cancellation until actual inference has ended.
const (
	TranscriptionQueued          TranscriptionStatus = "queued"
	TranscriptionRunning         TranscriptionStatus = "running"
	TranscriptionSucceeded       TranscriptionStatus = "succeeded"
	TranscriptionFailed          TranscriptionStatus = "failed"
	TranscriptionCancelRequested TranscriptionStatus = "cancel_requested"
	TranscriptionCancelled       TranscriptionStatus = "cancelled"
)

// TranscriptionJob exposes safe durable facts; recognized text is a separate query.
type TranscriptionJob struct {
	ID           uuid.UUID           `json:"id"`
	ProjectID    uuid.UUID           `json:"project_id"`
	Source       TranscriptionSource `json:"source"`
	Language     string              `json:"language"`
	Status       TranscriptionStatus `json:"status"`
	Stage        string              `json:"stage"`
	Progress     int                 `json:"progress"`
	Attempt      int                 `json:"attempt"`
	Revision     int64               `json:"revision"`
	ResultSHA256 *string             `json:"result_sha256" extensions:"x-nullable"`
	FailureCode  *string             `json:"failure_code" extensions:"x-nullable"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
}

// FrozenTranscription binds one original and requested language to one attempt.
type FrozenTranscription struct {
	Input    FrozenSource `json:"input"`
	Language string       `json:"language"`
}
