package application

// AudioSubtitle contains only supplier-returned text and millisecond offsets.
// It is persisted with private output evidence, never synthesized from a prompt.
type AudioSubtitle struct {
	Text      string          `json:"text"`
	Sentences []AudioSentence `json:"sentences"`
}

// AudioWord is a supplier-returned word with millisecond timing.
type AudioWord struct {
	StartTime int64  `json:"start_time"`
	EndTime   int64  `json:"end_time"`
	Text      string `json:"text"`
}

// AudioSentence groups timed words into one subtitle cue.
type AudioSentence struct {
	StartTime int64       `json:"start_time"`
	EndTime   int64       `json:"end_time"`
	Text      string      `json:"text"`
	Words     []AudioWord `json:"words"`
}

// Valid rejects unbounded text and timestamps outside the processed output.
func (s *AudioSubtitle) Valid(durationMS int64) bool {
	if s == nil {
		return true
	}
	if len(s.Text) > 60000 || len(s.Sentences) > 3000 || durationMS <= 0 || durationMS > 240000 {
		return false
	}
	previous := int64(0)
	total := len(s.Text)
	words := 0
	for _, sentence := range s.Sentences {
		if sentence.StartTime < previous || sentence.EndTime < sentence.StartTime || sentence.EndTime > durationMS || len(sentence.Words) > 3000 {
			return false
		}
		previous = sentence.StartTime
		total += len(sentence.Text)
		wordStart := sentence.StartTime
		for _, word := range sentence.Words {
			words++
			if words > 6000 {
				return false
			}
			if word.StartTime < wordStart || word.EndTime < word.StartTime || word.EndTime > sentence.EndTime {
				return false
			}
			wordStart = word.StartTime
			total += len(word.Text)
		}
	}
	return total <= 180000
}
