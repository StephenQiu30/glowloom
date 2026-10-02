package media_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	mediaflow "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/workflow"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
)

func documentProbe(t *testing.T, data []byte) error {
	t.Helper()
	file, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(data), "script.docx")
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	_, err = (mediaflow.FFUploadProber{}).Probe(t.Context(), file)
	return err
}

func TestDocumentContainerRejectsCRCEntryAndXMLBudgets(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"expanded part", documentDOCX(t, map[string]string{"word/large.xml": strings.Repeat("x", 16<<20+1)})},
		{"XML depth", documentDOCX(t, map[string]string{"word/extra.xml": strings.Repeat("<x>", 65) + strings.Repeat("</x>", 65)})},
		{"XML nodes", documentDOCX(t, map[string]string{"word/extra.xml": "<root>" + strings.Repeat("<x/>", 200000) + "</root>"})},
		{"DTD", documentDOCX(t, map[string]string{"word/extra.xml": `<!DOCTYPE x SYSTEM "https://must-not-fetch.invalid/secret"><x/>`})},
		{"macro content type", documentDOCX(t, map[string]string{"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.ms-word.document.macroEnabled.main+xml"/></Types>`})},
		{"duplicate XML attributes", documentDOCX(t, map[string]string{"word/extra.xml": `<x a="1" a="2"/>`})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := documentProbe(t, tc.data); !errors.Is(err, mediaapp.ErrUnsupportedUpload) {
				t.Fatal("unsafe container accepted", err)
			}
		})
	}
	base := documentDOCX(t, nil)
	reader, err := zip.NewReader(bytes.NewReader(base), int64(len(base)))
	if err != nil {
		t.Fatal(err)
	}
	var corrupt, duplicate bytes.Buffer
	for _, out := range []*bytes.Buffer{&corrupt, &duplicate} {
		writer := zip.NewWriter(out)
		for _, f := range reader.File {
			r, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(r)
			_ = r.Close()
			if err != nil {
				t.Fatal(err)
			}
			h := &zip.FileHeader{Name: f.Name, Method: zip.Store}
			part, err := writer.CreateHeader(h)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = part.Write(data); err != nil {
				t.Fatal(err)
			}
		}
		if out == &duplicate {
			w, err := writer.Create("word/document.xml")
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.WriteString(w, "<x/>")
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
	}
	data := corrupt.Bytes()
	i := bytes.Index(data, []byte("第一场"))
	if i < 0 {
		t.Fatal("missing stored fixture marker")
	}
	data[i] ^= 1
	if err := documentProbe(t, data); !errors.Is(err, mediaapp.ErrUnsupportedUpload) {
		t.Fatal("CRC mismatch accepted", err)
	}
	if err := documentProbe(t, duplicate.Bytes()); !errors.Is(err, mediaapp.ErrUnsupportedUpload) {
		t.Fatal("duplicate OOXML part accepted", err)
	}
}

func TestDocumentContainerDoesNotFetchExternalRelationships(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(503) }))
	defer server.Close()
	data := documentDOCX(t, map[string]string{"word/_rels/document.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="r1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/hyperlink" Target="` + server.URL + `" TargetMode="External"/></Relationships>`})
	if err := documentProbe(t, data); err != nil {
		t.Fatal("ordinary document relation without AV/network", err)
	}
	if calls.Load() != 0 {
		t.Fatal("document probe fetched external relationship")
	}
}

func TestDocumentContainerRejectsEntryAndCombinedExpansionBudgets(t *testing.T) {
	entries := make(map[string]string)
	for i := range 4094 {
		entries[fmt.Sprintf("word/custom/%d.xml", i)] = "<x/>"
	}
	if err := documentProbe(t, documentDOCX(t, entries)); !errors.Is(err, mediaapp.ErrUnsupportedUpload) {
		t.Fatal("entry budget overflow accepted", err)
	}
	large := strings.Repeat("x", 13<<20)
	parts := make(map[string]string)
	for i := range 5 {
		parts[fmt.Sprintf("word/media/image%d.bin", i)] = large
	}
	if err := documentProbe(t, documentDOCX(t, parts)); !errors.Is(err, mediaapp.ErrUnsupportedUpload) {
		t.Fatal("combined expansion overflow accepted", err)
	}
}

func TestDocumentReadBoundsRawFileAndLateBinaryBytes(t *testing.T) {
	if file, err := mediaapp.ReadUpload(t.Context(), io.LimitReader(strings.NewReader(strings.Repeat("x", 21<<20)), 21<<20), "too-large.txt"); !errors.Is(err, mediaapp.ErrUploadTooLarge) || file != nil {
		t.Fatal("raw source limit", err)
	}
	data := append(bytes.Repeat([]byte("x"), 1024), 0)
	file, err := mediaapp.ReadUpload(t.Context(), bytes.NewReader(data), "binary-late.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	if _, err := (mediaflow.FFUploadProber{}).Probe(t.Context(), file); !errors.Is(err, mediaapp.ErrUnsupportedUpload) {
		t.Fatal("late binary bytes accepted", err)
	}
}
