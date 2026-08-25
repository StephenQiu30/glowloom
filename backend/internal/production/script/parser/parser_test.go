package parser

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestAnalyzeRecognizesContinuousEpisodeMarkers(t *testing.T) {
	result := Analyze("第一集\n《警报前夜》\n内景·控制中心·夜\n沈岚：谁签的完成？\n\n第二集\n《公开日志》\n外景·堤岸·黎明\n潮水退去。\n")

	if result.Status != "deterministic" {
		t.Fatalf("status = %q", result.Status)
	}
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v", result.Issues)
	}
	markers := 0
	for _, block := range result.Blocks {
		if block.Kind == "episode_marker" {
			markers++
		}
	}
	if markers != 2 {
		t.Fatalf("episode markers = %d", markers)
	}
	if result.Blocks[0].Metadata["episode_number"] != 1 {
		t.Fatalf("first marker metadata = %#v", result.Blocks[0].Metadata)
	}
}

func TestAnalyzeRejectsBOMAndEpisodeNumberGap(t *testing.T) {
	result := Analyze("\ufeff第一集\n内容\n第三集\n内容\n")

	if result.Status != "rejected" {
		t.Fatalf("status = %q", result.Status)
	}
	codes := map[string]bool{}
	for _, issue := range result.Issues {
		codes[issue.Code] = true
	}
	if !codes["utf8_bom_not_allowed"] || !codes["number_gap"] {
		t.Fatalf("issue codes = %#v", codes)
	}
}

func TestDecodeDOCXReadsParagraphText(t *testing.T) {
	var document bytes.Buffer
	writer := zip.NewWriter(&document)
	part, err := writer.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, err = part.Write([]byte(`<?xml version="1.0"?><w:document xmlns:w="urn:test"><w:body><w:p><w:r><w:t>第一集</w:t></w:r></w:p><w:p><w:r><w:t>港口</w:t></w:r><w:r><w:t>警报</w:t></w:r></w:p></w:body></w:document>`))
	if err != nil {
		t.Fatal(err)
	}
	if err = writer.Close(); err != nil {
		t.Fatal(err)
	}

	text, err := DecodeDocument("application/vnd.openxmlformats-officedocument.wordprocessingml.document", document.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if text != "第一集\n港口警报" {
		t.Fatalf("decoded text = %q", text)
	}
}

func TestDecodeDocumentRejectsInvalidUTF8Markdown(t *testing.T) {
	if _, err := DecodeDocument("text/markdown", []byte{0xff}); err == nil {
		t.Fatal("DecodeDocument accepted invalid UTF-8")
	}
}
