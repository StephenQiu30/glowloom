package scripts_test

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/stephenqiu30/lanverse/backend/src/scripts"
)

func TestParseSourceDocumentSupportsMarkdownAndPlainText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fileName  string
		mediaType string
		input     []byte
		format    string
		text      string
	}{
		{
			name:      "markdown",
			fileName:  "全剧本.md",
			mediaType: "text/markdown; charset=utf-8",
			input:     []byte("\xef\xbb\xbf# 第1集 归途\r\n\r\n场景：海边码头\r\n林夏：出发。\r\n"),
			format:    "markdown",
			text:      "# 第1集 归途\n\n场景：海边码头\n林夏：出发。\n",
		},
		{
			name:      "plain text",
			fileName:  "全剧本.txt",
			mediaType: "text/plain",
			input:     []byte("第1集 归途\n场景：海边码头\n"),
			format:    "txt",
			text:      "第1集 归途\n场景：海边码头\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			document, err := scripts.ParseSourceDocument(test.fileName, test.mediaType, test.input)
			if err != nil {
				t.Fatalf("ParseSourceDocument() error = %v", err)
			}
			if document.Format != test.format {
				t.Fatalf("format = %q, want %q", document.Format, test.format)
			}
			if document.Text != test.text {
				t.Fatalf("text = %q, want %q", document.Text, test.text)
			}
			if document.OriginalHash == "" || document.TextHash == "" {
				t.Fatal("source hashes must be present")
			}
			if document.OriginalLength != len(test.input) {
				t.Fatalf("original length = %d, want %d", document.OriginalLength, len(test.input))
			}
		})
	}
}

func TestParseSourceDocumentExtractsDOCXParagraphsWithoutFormattingMarkup(t *testing.T) {
	t.Parallel()

	docx := buildDOCX(t, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>
<w:p><w:r><w:t>第1集 归途</w:t></w:r></w:p>
<w:p><w:r><w:t>场景：海边码头</w:t></w:r></w:p>
<w:p><w:r><w:t>林夏：</w:t><w:tab/><w:t>我们出发。</w:t><w:br/><w:t>现在。</w:t></w:r></w:p>
</w:body></w:document>`)

	document, err := scripts.ParseSourceDocument(
		"全剧本.docx",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		docx,
	)
	if err != nil {
		t.Fatalf("ParseSourceDocument() error = %v", err)
	}
	if document.Format != "docx" {
		t.Fatalf("format = %q, want docx", document.Format)
	}
	want := "第1集 归途\n场景：海边码头\n林夏：\t我们出发。\n现在。\n"
	if document.Text != want {
		t.Fatalf("text = %q, want %q", document.Text, want)
	}
	if document.ParagraphCount != 3 {
		t.Fatalf("paragraph count = %d, want 3", document.ParagraphCount)
	}
}

func TestParseSourceDocumentRejectsUnsupportedOrMalformedInput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		fileName  string
		mediaType string
		input     []byte
	}{
		{name: "unsupported PDF", fileName: "script.pdf", mediaType: "application/pdf", input: []byte("%PDF")},
		{name: "invalid UTF-8", fileName: "script.txt", mediaType: "text/plain", input: []byte{0xff, 0xfe, 0xfd}},
		{name: "fake DOCX", fileName: "script.docx", mediaType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document", input: []byte("not a zip")},
		{name: "empty source", fileName: "script.md", mediaType: "text/markdown", input: []byte(" \n\t")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := scripts.ParseSourceDocument(test.fileName, test.mediaType, test.input); err == nil {
				t.Fatal("ParseSourceDocument() should reject invalid source")
			}
		})
	}
}

func buildDOCX(t *testing.T, documentXML string) []byte {
	t.Helper()

	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, contents := range map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"word/document.xml":   documentXML,
	} {
		part, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(contents)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
