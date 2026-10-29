package application

import "slices"

// validateCreationMedia uses only frozen asset facts; client dimensions and
// durations cannot authorize a charged visual request.
func validateCreationMedia(input FreeQuoteItemInput, media []FreeQuoteMediaFact) error {
	if input.Mode == "omni_audio" {
		return validateOmniAudioMedia(input, media)
	}
	if input.Mode != "avatar" && input.Mode != "motion" {
		return nil
	}
	if input.Capability != "video.generate" || len(media) != 2 {
		return ErrFreeQuoteInputNotReady
	}
	for index, fact := range media {
		role := input.MediaInputs[index].Role
		switch {
		case role == "character_image":
			if !slices.Contains([]string{"image/jpeg", "image/png"}, fact.MIMEType) || fact.Width == nil || fact.Height == nil || *fact.Width <= 0 || *fact.Height <= 0 {
				return ErrFreeQuoteInputNotReady
			}
			if input.Mode == "avatar" && (*fact.Width >= 4096 || *fact.Height >= 4096 || fact.ByteSize >= 5000000) {
				return ErrFreeQuoteInputNotReady
			}
			if input.Mode == "motion" && (*fact.Width < 480 || *fact.Height < 480 || fact.ByteSize >= 4700000) {
				return ErrFreeQuoteInputNotReady
			}
		case input.Mode == "avatar" && role == "driving_audio":
			if fact.Kind != "audio" || fact.DurationMS == nil || *fact.DurationMS <= 0 || *fact.DurationMS >= 60000 {
				return ErrFreeQuoteInputNotReady
			}
		case input.Mode == "motion" && role == "driving_video":
			if !slices.Contains([]string{"video/mp4", "video/quicktime", "video/webm"}, fact.MIMEType) || fact.Width == nil || fact.Height == nil || *fact.Width < 200 || *fact.Height < 200 || max(*fact.Width, *fact.Height) > 2048 || min(*fact.Width, *fact.Height) > 1440 || fact.DurationMS == nil || *fact.DurationMS <= 0 || *fact.DurationMS > 30000 {
				return ErrFreeQuoteInputNotReady
			}
		default:
			return ErrFreeQuoteInputNotReady
		}
	}
	return nil
}
