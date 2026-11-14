package application

import (
	"bytes"
	"encoding/json"
	"io"
	"unicode/utf8"

	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
)

// MaxResultBytes bounds the complete extraction receipt, including provenance.
const MaxResultBytes = 32 << 20

// ValidateResult checks the same closed document schema and source facts on both
// sides of the process boundary. It grants no right to publish the content.
func ValidateResult(result Result) error {
	if _, err := result.Document.CanonicalJSON(); err != nil {
		return ErrExtractionFailed
	}
	if err := domain.ValidateSourceWarnings(result.Warnings); err != nil {
		return ErrExtractionFailed
	}
	switch result.Encoding {
	case "utf-8", "utf-8-bom", "utf-16le", "utf-16be", "gb18030", "docx-xml":
	default:
		return ErrExtractionFailed
	}
	plain, err := result.Document.PlainText()
	if err != nil || len(result.Mapping) > domain.MaxDocumentNodes {
		return ErrExtractionFailed
	}
	count := utf8.RuneCountInString(plain)
	for _, item := range result.Mapping {
		if item.Start < 0 || item.End < item.Start || item.End > count {
			return ErrExtractionFailed
		}
		if result.Encoding == "docx-xml" {
			if item.Part != "word/document.xml" || item.Paragraph == nil || *item.Paragraph < 0 || *item.Paragraph >= domain.MaxDocumentNodes || item.DecodedStart != nil || item.DecodedEnd != nil {
				return ErrExtractionFailed
			}
		} else if item.Part != "original" || item.Paragraph != nil || item.DecodedStart == nil || item.DecodedEnd == nil || *item.DecodedStart < 0 || *item.DecodedEnd < *item.DecodedStart || *item.DecodedEnd > domain.MaxScalarCount {
			return ErrExtractionFailed
		}
	}
	return nil
}

// EncodeResult produces one bounded JSON value after validating domain facts.
func EncodeResult(result Result) ([]byte, error) {
	if err := ValidateResult(result); err != nil {
		return nil, err
	}
	data, err := json.Marshal(result)
	if err != nil || len(data) > MaxResultBytes {
		return nil, ErrExtractionFailed
	}
	return data, nil
}

// DecodeResult refuses incomplete, unknown-field or multiple-value receipts.
func DecodeResult(data []byte) (Result, error) {
	var result Result
	if len(data) < 1 || len(data) > MaxResultBytes {
		return result, ErrExtractionFailed
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return Result{}, ErrExtractionFailed
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return Result{}, ErrExtractionFailed
	}
	if err := ValidateResult(result); err != nil {
		return Result{}, err
	}
	return result, nil
}
