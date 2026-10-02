package script_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"unicode/utf16"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/adapter/extract"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

func inputFile(t *testing.T, data []byte, mime string) *mediaapp.Downloaded {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "script-input-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	return &mediaapp.Downloaded{File: f, Size: int64(len(data)), MIMEType: mime, SHA256: domain.ContentSHA(data)}
}

func TestScriptExtractorTXTEncodingsPreserveCanonicalScalars(t *testing.T) {
	text := "第1集\r\n 😀é中 \n\n\n終"
	gbText := "第1集\r\n中文\n\n尾"
	gb, _, err := transform.Bytes(simplifiedchinese.GB18030.NewEncoder(), []byte(gbText))
	if err != nil {
		t.Fatal(err)
	}
	u16 := utf16.Encode([]rune(text))
	le, be := []byte{0xff, 0xfe}, []byte{0xfe, 0xff}
	for _, word := range u16 {
		le = binary.LittleEndian.AppendUint16(le, word)
		be = binary.BigEndian.AppendUint16(be, word)
	}
	for _, tc := range []struct {
		name string
		data []byte
		want string
	}{
		{"UTF8", []byte(text), domain.NormalizeText(text)},
		{"UTF8BOM", append([]byte{0xef, 0xbb, 0xbf}, []byte(text)...), domain.NormalizeText(text)},
		{"UTF16LE", le, domain.NormalizeText(text)}, {"UTF16BE", be, domain.NormalizeText(text)},
		{"GB18030", gb, domain.NormalizeText(gbText)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := extract.NewExtractor().Extract(context.Background(), inputFile(t, tc.data, "text/plain"))
			if err != nil {
				t.Fatal(err)
			}
			plain, err := result.Document.PlainText()
			if err != nil || plain != tc.want || len(result.Mapping) == 0 {
				t.Fatalf("encoding loss: %q want %q: %v", plain, tc.want, err)
			}
		})
	}
}

func TestScriptExtractorRejectsReplacementAndChangedBytes(t *testing.T) {
	for _, data := range [][]byte{{0xff}, {0x81}, {0x81, 0x30, 0x81}, {0xff, 0xfe, 0x00, 0xd8}, {0xfe, 0xff, 0xdc, 0x00}, {0, 1}} {
		if _, err := extract.NewExtractor().Extract(context.Background(), inputFile(t, data, "text/plain")); !errors.Is(err, scriptapp.ErrExtractionFailed) {
			t.Errorf("bad encoding accepted %x: %v", data, err)
		}
	}
	f := inputFile(t, []byte("正文"), "text/plain")
	f.SHA256 = strings.Repeat("0", 64)
	if _, err := extract.NewExtractor().Extract(context.Background(), f); !errors.Is(err, scriptapp.ErrExtractionFailed) {
		t.Fatal("accepted changed frozen bytes")
	}
}

func docxFile(t *testing.T, body string) []byte {
	t.Helper()
	var raw bytes.Buffer
	w := zip.NewWriter(&raw)
	parts := []struct{ name, content string }{
		{"[Content_Types].xml", `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`},
		{"_rels/.rels", `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml" Id="rId1"/></Relationships>`},
		{"word/document.xml", `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` + body + `</w:body></w:document>`},
	}
	for _, part := range parts {
		entry, err := w.Create(part.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, part.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return raw.Bytes()
}

func TestScriptExtractorDOCXPreservesParagraphFormattingAndWarnings(t *testing.T) {
	body := `<w:p><w:pPr><w:pStyle w:val="Heading2"/><w:jc w:val="center"/></w:pPr><w:r><w:t>第1集</w:t></w:r></w:p><w:p><w:r><w:rPr><w:b/><w:i/><w:u w:val="single"/><w:color w:val="D97706"/><w:highlight w:val="yellow"/></w:rPr><w:t xml:space="preserve"> 人😀 </w:t><w:br/><w:t>尾</w:t></w:r><w:r><w:drawing/></w:r></w:p><w:tbl><w:tr><w:tc><w:p><w:r><w:t>表内</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`
	result, err := extract.NewExtractor().Extract(context.Background(), inputFile(t, docxFile(t, body), "application/vnd.openxmlformats-officedocument.wordprocessingml.document"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := result.Document.PlainText()
	if err != nil || plain != "第1集\n\n 人😀 \n尾\n\n表内" || result.Document.Content[0].Attrs.Level != 2 || result.Document.Content[0].Attrs.Align != "center" {
		t.Fatalf("DOCX body loss: %q %+v %v", plain, result.Document, err)
	}
	encoded, err := result.Document.CanonicalJSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, mark := range []string{"bold", "italic", "underline", "textColor", "highlight"} {
		if !strings.Contains(string(encoded), `"type":"`+mark+`"`) {
			t.Fatalf("missing DOCX format %s", mark)
		}
	}
	if len(result.Warnings) < 2 || len(result.Mapping) != 3 {
		t.Fatalf("hidden image/table loss or missing source map: %+v", result)
	}
}

func TestScriptExtractorDOCXFailureAndCancellation(t *testing.T) {
	for _, body := range []string{`<w:p><w:r><w:t>broken`, `<!DOCTYPE x [<!ENTITY boom SYSTEM "http://example.test/a">]><w:p><w:r><w:t>&boom;</w:t></w:r></w:p>`} {
		if _, err := extract.NewExtractor().Extract(context.Background(), inputFile(t, docxFile(t, body), "application/vnd.openxmlformats-officedocument.wordprocessingml.document")); !errors.Is(err, scriptapp.ErrExtractionFailed) {
			t.Fatalf("invalid DOCX accepted: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := extract.NewExtractor().Extract(ctx, inputFile(t, []byte("内容"), "text/plain")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel ignored: %v", err)
	}
}

func TestScriptExtractorDOCXRejectsSpoofedMainPartRelationship(t *testing.T) {
	original := docxFile(t, `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>正文</w:t></w:r></w:p></w:body></w:document>`)
	archive, err := zip.NewReader(bytes.NewReader(original), int64(len(original)))
	if err != nil {
		t.Fatal(err)
	}
	for _, spoof := range []string{"content_type_comment", "relationship_comment"} {
		t.Run(spoof, func(t *testing.T) {
			var buffer bytes.Buffer
			writer := zip.NewWriter(&buffer)
			for _, part := range archive.File {
				reader, err := part.Open()
				if err != nil {
					t.Fatal(err)
				}
				data, err := io.ReadAll(reader)
				if err != nil {
					t.Fatal(err)
				}
				if err := reader.Close(); err != nil {
					t.Fatal(err)
				}
				if spoof == "content_type_comment" && part.Name == "[Content_Types].xml" {
					data = []byte(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><!-- application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml --></Types>`)
				}
				if spoof == "relationship_comment" && part.Name == "_rels/.rels" {
					data = []byte(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><!-- word/document.xml --></Relationships>`)
				}
				entry, err := writer.Create(part.Name)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := entry.Write(data); err != nil {
					t.Fatal(err)
				}
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			file := inputFile(t, buffer.Bytes(), "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
			if _, err := extract.NewExtractor().Extract(t.Context(), file); err == nil {
				t.Fatal("spoofed standard Word container accepted")
			}
		})
	}
}
