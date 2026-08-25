package parser

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

const docxMIMEType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"

var (
	episodeMarker = regexp.MustCompile(`^(?:#{1,6}\s*)?第([〇零一二三四五六七八九十百两0-9]+)集(?:\s*)$`)
	dialogueLine  = regexp.MustCompile(`^[^：:\n]{1,30}[：:]`)
)

type Block struct {
	Position, SourceStart, SourceEnd int
	Kind                             string
	TextHashInput                    string
	Metadata                         map[string]any
}

type Issue struct {
	Position, SourceStart, SourceEnd, LineNumber, ColumnNumber int
	Code, Severity, NextAction                                 string
	Details                                                    map[string]any
}

type Analysis struct {
	NormalizedText   string
	NormalizationMap map[string]any
	Blocks           []Block
	Issues           []Issue
	Status           string
}

func DecodeDocument(mimeType string, contents []byte) (string, error) {
	switch mimeType {
	case "text/markdown", "text/plain":
		if !utf8.Valid(contents) {
			return "", errors.New("document text is not valid UTF-8")
		}
		return string(contents), nil
	case docxMIMEType:
		return decodeDOCX(contents)
	default:
		return "", fmt.Errorf("unsupported document media type %q", mimeType)
	}
}

func decodeDOCX(contents []byte) (string, error) {
	archive, err := zip.NewReader(bytes.NewReader(contents), int64(len(contents)))
	if err != nil {
		return "", fmt.Errorf("open DOCX archive: %w", err)
	}
	for _, file := range archive.File {
		if file.Name != "word/document.xml" {
			continue
		}
		stream, openErr := file.Open()
		if openErr != nil {
			return "", fmt.Errorf("open DOCX document part: %w", openErr)
		}
		text, decodeErr := decodeWordXML(io.LimitReader(stream, 64<<20))
		closeErr := stream.Close()
		if decodeErr != nil {
			return "", decodeErr
		}
		if closeErr != nil {
			return "", fmt.Errorf("close DOCX document part: %w", closeErr)
		}
		return text, nil
	}
	return "", errors.New("DOCX document part is missing")
}

func decodeWordXML(reader io.Reader) (string, error) {
	decoder := xml.NewDecoder(reader)
	var paragraphs []string
	var paragraph strings.Builder
	inParagraph := false
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("decode DOCX document XML: %w", err)
		}
		switch value := token.(type) {
		case xml.StartElement:
			switch value.Name.Local {
			case "p":
				inParagraph = true
				paragraph.Reset()
			case "t":
				if inParagraph {
					var text string
					if err = decoder.DecodeElement(&text, &value); err != nil {
						return "", fmt.Errorf("decode DOCX text: %w", err)
					}
					paragraph.WriteString(text)
				}
			case "tab":
				if inParagraph {
					paragraph.WriteByte('\t')
				}
			case "br":
				if inParagraph {
					paragraph.WriteByte('\n')
				}
			}
		case xml.EndElement:
			if value.Name.Local == "p" && inParagraph {
				paragraphs = append(paragraphs, paragraph.String())
				inParagraph = false
			}
		}
	}
	return strings.Join(paragraphs, "\n"), nil
}

func Analyze(rawText string) Analysis {
	normalized := strings.ReplaceAll(strings.ReplaceAll(rawText, "\r\n", "\n"), "\r", "\n")
	result := Analysis{
		NormalizedText: normalized,
		NormalizationMap: map[string]any{
			"line_endings":          "lf",
			"source_codepoints":     utf8.RuneCountInString(rawText),
			"normalized_codepoints": utf8.RuneCountInString(normalized),
		},
	}
	if strings.HasPrefix(normalized, "\ufeff") {
		result.Issues = append(result.Issues, Issue{Code: "utf8_bom_not_allowed", Severity: "blocking", SourceStart: 0, SourceEnd: 1, LineNumber: 1, ColumnNumber: 1, NextAction: "remove_utf8_bom", Details: map[string]any{}})
	}

	lines := strings.SplitAfter(normalized, "\n")
	offset := 0
	seenMarker := false
	markerNumbers := make([]int, 0)
	markerBlockIndexes := make([]int, 0)
	for lineIndex, segment := range lines {
		if segment == "" && lineIndex == len(lines)-1 {
			continue
		}
		line := strings.TrimSuffix(segment, "\n")
		lineRunes := []rune(line)
		start := offset
		end := start + len(lineRunes)
		offset += utf8.RuneCountInString(segment)
		trimmed := strings.TrimSpace(line)
		kind := classify(trimmed, seenMarker)
		metadata := map[string]any{"line_number": lineIndex + 1}
		if matches := episodeMarker.FindStringSubmatch(trimmed); matches != nil {
			number, ok := episodeNumber(matches[1])
			if ok {
				kind = "episode_marker"
				metadata["episode_number"] = number
				seenMarker = true
				markerNumbers = append(markerNumbers, number)
				markerBlockIndexes = append(markerBlockIndexes, len(result.Blocks))
			}
		}
		result.Blocks = append(result.Blocks, Block{Position: len(result.Blocks) + 1, Kind: kind, SourceStart: start, SourceEnd: end, TextHashInput: line, Metadata: metadata})
	}

	if len(markerNumbers) == 0 {
		result.Issues = append(result.Issues, Issue{Position: len(result.Issues) + 1, Code: "no_marker", Severity: "warning", SourceStart: 0, SourceEnd: 0, LineNumber: 1, ColumnNumber: 1, NextAction: "generate_episode_plan", Details: map[string]any{}})
	} else {
		seen := map[int]bool{}
		for index, number := range markerNumbers {
			block := result.Blocks[markerBlockIndexes[index]]
			if seen[number] {
				result.addMarkerIssue("duplicate_number", "blocking", "renumber_episode_markers", block, number, index+1)
			} else if index > 0 && number < markerNumbers[index-1] {
				result.addMarkerIssue("number_out_of_order", "blocking", "reorder_episode_markers", block, number, index+1)
			} else if number != index+1 {
				result.addMarkerIssue("number_gap", "blocking", "renumber_episode_markers", block, number, index+1)
			}
			seen[number] = true
		}
		if len(markerNumbers) > 100 {
			block := result.Blocks[markerBlockIndexes[100]]
			result.addMarkerIssue("episode_limit_exceeded", "blocking", "reduce_episode_count", block, markerNumbers[100], 101)
		}
		for index, markerIndex := range markerBlockIndexes {
			next := len(result.Blocks)
			if index+1 < len(markerBlockIndexes) {
				next = markerBlockIndexes[index+1]
			}
			nonEmpty := false
			for _, block := range result.Blocks[markerIndex+1 : next] {
				if block.Kind != "separator" {
					nonEmpty = true
					break
				}
			}
			if !nonEmpty {
				block := result.Blocks[markerIndex]
				result.addMarkerIssue("empty_episode", "blocking", "renumber_episode_markers", block, markerNumbers[index], index+1)
			}
		}
	}

	for _, block := range result.Blocks {
		if block.Kind == "preamble" && strings.TrimSpace(block.TextHashInput) != "" {
			result.Issues = append(result.Issues, Issue{Position: len(result.Issues) + 1, Code: "preamble_requires_decision", Severity: "warning", SourceStart: block.SourceStart, SourceEnd: block.SourceEnd, LineNumber: lineNumber(block.Metadata), ColumnNumber: 1, NextAction: "resolve_preamble", Details: map[string]any{}})
			break
		}
	}

	result.Status = "deterministic"
	for _, issue := range result.Issues {
		if issue.Severity == "blocking" {
			result.Status = "rejected"
			break
		}
		if issue.Code == "no_marker" || issue.Code == "preamble_requires_decision" {
			result.Status = "ai_candidate_required"
		}
	}
	for index := range result.Issues {
		result.Issues[index].Position = index + 1
	}
	return result
}

func (result *Analysis) addMarkerIssue(code, severity, nextAction string, block Block, actual, expected int) {
	result.Issues = append(result.Issues, Issue{Code: code, Severity: severity, SourceStart: block.SourceStart, SourceEnd: block.SourceEnd, LineNumber: lineNumber(block.Metadata), ColumnNumber: 1, NextAction: nextAction, Details: map[string]any{"actual": actual, "expected": expected}})
}

func classify(trimmed string, seenMarker bool) string {
	if trimmed == "" {
		return "separator"
	}
	if !seenMarker {
		return "preamble"
	}
	upper := strings.ToUpper(trimmed)
	if strings.HasPrefix(trimmed, "内景") || strings.HasPrefix(trimmed, "外景") || strings.HasPrefix(upper, "INT.") || strings.HasPrefix(upper, "EXT.") {
		return "scene_heading"
	}
	if dialogueLine.MatchString(trimmed) {
		return "dialogue"
	}
	if strings.HasPrefix(trimmed, "旁白") || strings.HasPrefix(trimmed, "画外音") {
		return "narration"
	}
	return "action"
}

func episodeNumber(value string) (int, bool) {
	if number, err := strconv.Atoi(value); err == nil && number > 0 {
		return number, true
	}
	values := map[rune]int{'〇': 0, '零': 0, '一': 1, '二': 2, '两': 2, '三': 3, '四': 4, '五': 5, '六': 6, '七': 7, '八': 8, '九': 9}
	total, current := 0, 0
	for _, character := range value {
		switch character {
		case '十':
			if current == 0 {
				current = 1
			}
			total += current * 10
			current = 0
		case '百':
			if current == 0 {
				current = 1
			}
			total += current * 100
			current = 0
		default:
			digit, ok := values[character]
			if !ok {
				return 0, false
			}
			current = current*10 + digit
		}
	}
	total += current
	return total, total > 0
}

func lineNumber(metadata map[string]any) int {
	if number, ok := metadata["line_number"].(int); ok {
		return number
	}
	return 1
}
