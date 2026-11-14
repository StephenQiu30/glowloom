package application

import (
	"slices"
	"strings"
	"unicode/utf8"
)

func validateOmniAudioMedia(in FreeQuoteItemInput, facts []FreeQuoteMediaFact) error {
	if in.Capability != "audio.generate" || len(facts) != len(in.MediaInputs) || len(facts) > 3 || strings.TrimSpace(in.Prompt) == "" || utf8.RuneCountInString(in.Prompt) > 3000 {
		return ErrFreeQuoteInputNotReady
	}
	kind := ""
	for i, f := range facts {
		if f.ByteSize <= 0 || f.ByteSize > 10000000 || kind != "" && kind != f.Kind {
			return ErrFreeQuoteInputNotReady
		}
		kind = f.Kind
		switch {
		case f.Kind == "image" && in.MediaInputs[i].Role == "reference_image" && len(facts) == 1:
			if f.ByteSize < 1024 || !slices.Contains([]string{"image/jpeg", "image/png", "image/webp"}, f.MIMEType) {
				return ErrFreeQuoteInputNotReady
			}
		case f.Kind == "audio" && in.MediaInputs[i].Role == "reference_audio":
			if !slices.Contains([]string{"audio/mpeg", "audio/wav", "audio/x-wav", "audio/ogg"}, f.MIMEType) || f.DurationMS == nil || *f.DurationMS <= 0 || *f.DurationMS > 30000 {
				return ErrFreeQuoteInputNotReady
			}
		default:
			return ErrFreeQuoteInputNotReady
		}
	}
	return nil
}
