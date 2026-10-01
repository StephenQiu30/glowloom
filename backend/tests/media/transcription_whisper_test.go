package media_test

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	whisper "github.com/StephenQiu30/lanverse/backend/internal/mediatool/adapter/whisper"
	toolapp "github.com/StephenQiu30/lanverse/backend/internal/mediatool/application"
)

func transcriptionPCM(t *testing.T) *mediaapp.Downloaded {
	t.Helper()
	data := make([]byte, 44+32000)
	copy(data, "RIFF")
	binary.LittleEndian.PutUint32(data[4:], uint32(len(data)-8))
	copy(data[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(data[16:], 16)
	binary.LittleEndian.PutUint16(data[20:], 1)
	binary.LittleEndian.PutUint16(data[22:], 1)
	binary.LittleEndian.PutUint32(data[24:], 16000)
	binary.LittleEndian.PutUint32(data[28:], 32000)
	binary.LittleEndian.PutUint16(data[32:], 2)
	binary.LittleEndian.PutUint16(data[34:], 16)
	copy(data[36:], "data")
	binary.LittleEndian.PutUint32(data[40:], 32000)
	p := filepath.Join(t.TempDir(), "test.wav")
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	return &mediaapp.Downloaded{File: f, Size: int64(len(data)), MIMEType: "audio/wav"}
}
func TestWhisperNativeMultipartAndStrictTiming(t *testing.T) {
	wav := transcriptionPCM(t)
	defer func() { _ = wav.Close() }()
	response := `{"task":"transcribe","language":"chinese","duration":1,"segments":[{"start":0.01,"end":0.9,"text":" 真实字幕\r\n第二行 "}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/inference" {
			t.Error("wrong native endpoint")
		}
		if err := r.ParseMultipartForm(1 << 20); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		defer func() { _ = r.MultipartForm.RemoveAll() }()
		if r.FormValue("response_format") != "verbose_json" || r.FormValue("language") != "zh" || r.FormValue("translate") != "false" || r.FormValue("no_timestamps") != "false" {
			t.Error("wrong recognition parameters")
		}
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Error(err)
		} else {
			data, _ := io.ReadAll(file)
			_ = file.Close()
			if len(data) != 32044 {
				t.Errorf("multipart lost WAV bytes: %d", len(data))
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, response)
	}))
	defer server.Close()
	client, err := whisper.NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	draft, err := client.Transcribe(t.Context(), wav, "zh", 1000)
	if err != nil || draft.Language != "chinese" || len(draft.Segments) != 1 || draft.Segments[0].StartMS != 10 || draft.Segments[0].EndMS != 900 || draft.Segments[0].Text != "真实字幕\n第二行" {
		t.Fatalf("actual native cue: %+v %v", draft, err)
	}
	for _, invalid := range []string{
		`{"language":"chinese","duration":1,"segments":[{"end":0.9,"text":"x"}]}`,
		`{"language":"chinese","duration":1,"segments":[{"start":0,"text":"x"}]}`,
		`{"language":"chinese","duration":1,"segments":[{"start":-0.1,"end":0.9,"text":"x"}]}`,
		`{"language":"chinese","duration":1,"segments":[{"start":0.9,"end":0.1,"text":"x"}]}`,
		`{"language":"chinese","duration":1,"segments":[{"start":0,"end":1.01,"text":"x"}]}`,
		`{"language":"chinese","duration":2,"segments":[{"start":0,"end":1,"text":"x"}]}`,
		`{"language":"chinese","duration":1,"segments":[{"start":0,"end":0.8,"text":"x"},{"start":0.7,"end":1,"text":"y"}]}`,
		`{"language":"chinese","duration":1,"segments":[{"start":0,"end":1,"text":"\u0000"}]}`,
		`{"language":"chinese","duration":1,"segments":[{"start":0,"end":1,"text":"x"}]} {}`,
	} {
		response = invalid
		if _, err := client.Transcribe(t.Context(), wav, "zh", 1000); err == nil {
			t.Errorf("accepted invalid native timing: %s", invalid)
		}
	}
	response = `{"language":"english","duration":1,"segments":[]}`
	if _, err := client.Transcribe(t.Context(), wav, "zh", 1000); !errors.Is(err, toolapp.ErrNoSpeech) {
		t.Fatalf("no speech fabricated cues: %v", err)
	}
	for _, endpoint := range []string{"http://example.com", "http://127.0.0.1:19282/other", "http://127.0.0.1:19282?x=1", "http://user@127.0.0.1:19282"} {
		if _, err := whisper.NewClient(endpoint, server.Client()); err == nil {
			t.Errorf("accepted unsafe endpoint %s", endpoint)
		}
	}
}

func TestWhisperDoesNotFollowRedirectOrExposeResponseBody(t *testing.T) {
	wav := transcriptionPCM(t)
	defer func() { _ = wav.Close() }()
	entered := false
	redirected := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { entered = true; w.WriteHeader(200) }))
	defer redirected.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", redirected.URL)
		w.WriteHeader(307)
		_, _ = fmt.Fprint(w, "private transcript details")
	}))
	defer server.Close()
	client, err := whisper.NewClient(server.URL, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Transcribe(t.Context(), wav, "en", 1000); err == nil || entered {
		t.Fatalf("followed redirect: %v %v", entered, err)
	}
}
