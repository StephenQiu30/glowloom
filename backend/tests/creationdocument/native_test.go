package creationdocument_test

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/adapter/native"
	"github.com/StephenQiu30/glowloom-server/backend/internal/creation/document/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

const docxMIME = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

func nativeOriginal(t *testing.T, data []byte, mime string) *application.File {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "document-*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return &application.File{File: file, Size: int64(len(data)), SHA256: domain.ContentSHA(data), MIMEType: mime}
}

func nativeWord(t *testing.T, extraName string, extraBytes int64) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	parts := []struct{ name, content string }{
		{"[Content_Types].xml", `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`},
		{"_rels/.rels", `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml" Id="rId1"/></Relationships>`},
		{"word/document.xml", `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>正文😀</w:t></w:r></w:p></w:body></w:document>`},
	}
	for _, part := range parts {
		entry, err := writer.Create(part.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, part.content); err != nil {
			t.Fatal(err)
		}
	}
	if extraName != "" {
		entry, err := writer.Create(extraName)
		if err != nil {
			t.Fatal(err)
		}
		chunk := bytes.Repeat([]byte{'x'}, 64*1024)
		for extraBytes > 0 {
			n := min(int64(len(chunk)), extraBytes)
			if _, err := entry.Write(chunk[:n]); err != nil {
				t.Fatal(err)
			}
			extraBytes -= n
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func assertOriginalUnchanged(t *testing.T, file *application.File, name string, offset int64) {
	t.Helper()
	if file.File == nil {
		t.Fatal("native extraction closed caller-owned original")
	}
	if _, err := os.Stat(name); err != nil {
		t.Fatalf("native extraction removed caller-owned original: %v", err)
	}
	position, err := file.File.Seek(0, io.SeekCurrent)
	if err != nil || position != offset {
		t.Fatalf("original cursor=%d error=%v, want %d", position, err, offset)
	}
}

func TestNativeDocumentExtractionBorrowsOriginalAndPreservesCursor(t *testing.T) {
	var extractor application.Extractor = native.NewExtractor()
	for _, tc := range []struct {
		name, mime, encoding, text string
		data                       []byte
	}{
		{"TXT", "text/plain", "utf-8", "第1集\n\n正文😀", []byte("第1集\r\n\r\n正文😀")},
		{"DOCX", docxMIME, "docx-xml", "正文😀", nativeWord(t, "", 0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := nativeOriginal(t, tc.data, tc.mime)
			const offset = 2
			if _, err := file.File.Seek(offset, io.SeekStart); err != nil {
				t.Fatal(err)
			}
			result, err := extractor.Extract(t.Context(), file)
			if err != nil {
				t.Fatal(err)
			}
			text, err := result.Document.PlainText()
			if err != nil || text != tc.text || result.Encoding != tc.encoding || len(result.Mapping) == 0 {
				t.Fatalf("extracted text=%q encoding=%q mapping=%v error=%v", text, result.Encoding, result.Mapping, err)
			}
			assertOriginalUnchanged(t, file, file.File.Name(), offset)
		})
	}
}

func TestNativeDocumentExtractionRejectsInvalidReceiptsWithoutTakingOwnership(t *testing.T) {
	if application.MaxBytes != 20<<20 {
		t.Fatalf("raw document limit=%d, want 20 MiB rather than rich JSON's 8 MiB", application.MaxBytes)
	}
	for _, tc := range []struct {
		name   string
		change func(*application.File)
	}{
		{"claimed size too short", func(f *application.File) { f.Size-- }},
		{"claimed size too long", func(f *application.File) { f.Size++ }},
		{"claimed size over hard limit", func(f *application.File) { f.Size = application.MaxBytes + 1 }},
		{"empty size", func(f *application.File) { f.Size = 0 }},
		{"wrong checksum", func(f *application.File) { f.SHA256 = strings.Repeat("0", 64) }},
		{"unsupported MIME", func(f *application.File) { f.MIMEType = "text/html" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := nativeOriginal(t, []byte("正文😀"), "text/plain")
			tc.change(file)
			name := file.File.Name()
			position, err := file.File.Seek(1, io.SeekStart)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := native.NewExtractor().Extract(t.Context(), file); !errors.Is(err, application.ErrExtractionFailed) {
				t.Fatalf("invalid original accepted or lost sentinel: %v", err)
			}
			assertOriginalUnchanged(t, file, name, position)
		})
	}
	if _, err := native.NewExtractor().Extract(t.Context(), nil); !errors.Is(err, application.ErrExtractionFailed) {
		t.Fatalf("nil original error=%v", err)
	}
}

func TestNativeDocumentExtractionPreservesReadFailureChain(t *testing.T) {
	file := nativeOriginal(t, []byte("正文😀"), "text/plain")
	if err := file.File.Close(); err != nil {
		t.Fatal(err)
	}
	_, err := native.NewExtractor().Extract(t.Context(), file)
	if !errors.Is(err, application.ErrExtractionFailed) || !errors.Is(err, os.ErrClosed) {
		t.Fatalf("closed original lost extraction or I/O cause: %v", err)
	}
}

func TestNativeDocumentExtractionPreservesCancellation(t *testing.T) {
	file := nativeOriginal(t, []byte("正文😀"), "text/plain")
	name := file.File.Name()
	position, err := file.File.Seek(1, io.SeekStart)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := native.NewExtractor().Extract(ctx, file); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled extraction error=%v", err)
	}
	assertOriginalUnchanged(t, file, name, position)
}

func TestNativeDocumentExtractionEnforcesScalarAndUTF8Boundaries(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
		fail bool
	}{
		{"maximum valid Unicode scalars", []byte(strings.Repeat("😀", domain.MaxScalarCount)), false},
		{"one scalar over limit", []byte(strings.Repeat("a", domain.MaxScalarCount+1)), true},
		{"valid UTF8 BOM", []byte("\xef\xbb\xbf正文😀"), false},
		{"truncated UTF8 after BOM", []byte{0xef, 0xbb, 0xbf, 0xf0, 0x9f, 0x98}, true},
		{"UTF16 unpaired high surrogate", []byte{0xff, 0xfe, 0x00, 0xd8}, true},
		{"embedded NUL", []byte("正文\x00尾"), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := native.NewExtractor().Extract(t.Context(), nativeOriginal(t, tc.data, "text/plain"))
			if tc.fail && !errors.Is(err, application.ErrExtractionFailed) || !tc.fail && err != nil {
				t.Fatalf("UTF/scalar boundary error=%v, want failure=%v", err, tc.fail)
			}
		})
	}
}

func TestNativeDocumentExtractionRejectsUnsafeWordArchives(t *testing.T) {
	for _, tc := range []struct {
		name, part string
		size       int64
	}{
		{"expanded part over 16 MiB", "word/media/large.bin", (16 << 20) + 1},
		{"relative traversal", "../outside.bin", 1},
		{"absolute path", "/outside.bin", 1},
		{"macro payload", "word/vbaProject.bin", 1},
		{"duplicate main part", "word/document.xml", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := nativeOriginal(t, nativeWord(t, tc.part, tc.size), docxMIME)
			if _, err := native.NewExtractor().Extract(t.Context(), file); !errors.Is(err, application.ErrExtractionFailed) {
				t.Fatalf("unsafe DOCX error=%v", err)
			}
		})
	}
}
