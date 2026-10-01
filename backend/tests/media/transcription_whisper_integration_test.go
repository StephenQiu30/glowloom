package media_test

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
	"unicode"

	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	toolff "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/ffmpeg"
	whisper "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/whisper"
)

func TestWhisperRealOfficialSpeechAndSyntheticChinese(t *testing.T) {
	endpoint := os.Getenv("LV_TEST_WHISPER_BASE_URL")
	if endpoint == "" {
		if os.Getenv("LV_TEST_WHISPER_REQUIRED") == "1" {
			t.Fatal("required actual Whisper service is missing")
		}
		t.Skip("actual Whisper service was not configured")
	}
	client, err := whisper.NewClient(endpoint, &http.Client{Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	prep, err := toolff.NewAudioPreprocessor(mediaflow.FFProber{})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		name, path, language string
		duration             int64
		phrase               string
	}{{"official", os.Getenv("LV_TEST_WHISPER_SAMPLE"), "en", 11000, "ask not"}, {"synthetic Chinese", os.Getenv("LV_TEST_WHISPER_CHINESE_SAMPLE"), "zh", 5560, ""}} {
		t.Run(item.name, func(t *testing.T) {
			if item.path == "" {
				t.Fatal("required actual speech fixture is missing")
			}
			f, err := os.Open(item.path)
			if err != nil {
				t.Fatal(err)
			}
			info, err := f.Stat()
			if err != nil {
				_ = f.Close()
				t.Fatal(err)
			}
			// Input fixtures belong to the task; do not remove them via Downloaded.Close.
			defer func() { _ = f.Close() }()
			pcm, err := prep.Prepare(t.Context(), &mediaapp.Downloaded{File: f, Size: info.Size(), MIMEType: "audio/x-wav"}, item.duration)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = pcm.File.Close() }()
			draft, err := client.Transcribe(t.Context(), pcm.File, item.language, pcm.DurationMS)
			if err != nil {
				t.Fatal(err)
			}
			text := strings.Builder{}
			hasChinese := false
			for _, cue := range draft.Segments {
				text.WriteString(cue.Text)
				for _, r := range cue.Text {
					hasChinese = hasChinese || unicode.Is(unicode.Han, r)
				}
			}
			if item.language == "en" && (draft.Language != "english" || !strings.Contains(strings.ToLower(text.String()), item.phrase)) {
				t.Fatalf("actual English recognition missing expected spoken phrase: %+v", draft)
			}
			// A base model can misrecognize words. This validates actual Chinese output
			// and timings, not transcription accuracy or a passed human review.
			if item.language == "zh" && (draft.Language != "chinese" || !hasChinese) {
				t.Fatalf("actual Chinese recognition absent: %+v", draft)
			}
			if _, err := draft.SRT(); err != nil {
				t.Fatal(err)
			}
		})
	}
}
