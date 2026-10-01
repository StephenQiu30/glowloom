package media_test

import (
	"strings"
	"testing"

	tooldomain "github.com/StephenQiu30/lanverse/backend/internal/mediatool/domain"
)

func TestTranscriptionDraftValidatesAndPreservesExactSRT(t *testing.T) {
	draft := tooldomain.Transcript{Version: 1, Language: "chinese", DurationMS: 2500, Segments: []tooldomain.SubtitleSegment{{StartMS: 10, EndMS: 1200, Text: "你好，Lanverse。\n第二行"}, {StartMS: 1300, EndMS: 2500, Text: "字幕测试"}}}
	if err := draft.Validate(); err != nil {
		t.Fatal(err)
	}
	srt, err := draft.SRT()
	if err != nil || srt != "1\n00:00:00,010 --> 00:00:01,200\n你好，Lanverse。\n第二行\n\n2\n00:00:01,300 --> 00:00:02,500\n字幕测试\n\n" {
		t.Fatalf("exact subtitle cues: %q %v", srt, err)
	}
	cases := []tooldomain.Transcript{
		{Version: 1, Language: "chinese", DurationMS: 2500},
		{Version: 1, Language: "chinese", DurationMS: 2500, Segments: []tooldomain.SubtitleSegment{{StartMS: -1, EndMS: 100, Text: "x"}}},
		{Version: 1, Language: "chinese", DurationMS: 2500, Segments: []tooldomain.SubtitleSegment{{StartMS: 100, EndMS: 100, Text: "x"}}},
		{Version: 1, Language: "chinese", DurationMS: 2500, Segments: []tooldomain.SubtitleSegment{{StartMS: 0, EndMS: 2501, Text: "x"}}},
		{Version: 1, Language: "chinese", DurationMS: 2500, Segments: []tooldomain.SubtitleSegment{{StartMS: 0, EndMS: 1500, Text: "x"}, {StartMS: 1400, EndMS: 2500, Text: "y"}}},
		{Version: 1, Language: "chinese", DurationMS: 2500, Segments: []tooldomain.SubtitleSegment{{StartMS: 0, EndMS: 1000, Text: "\xff"}}},
		{Version: 1, Language: "chinese", DurationMS: 2500, Segments: []tooldomain.SubtitleSegment{{StartMS: 0, EndMS: 1000, Text: " \n "}}},
		{Version: 1, Language: "chinese", DurationMS: 2500, Segments: []tooldomain.SubtitleSegment{{StartMS: 0, EndMS: 1000, Text: strings.Repeat("x", 32769)}}},
	}
	for i, invalid := range cases {
		if invalid.Validate() == nil {
			t.Errorf("accepted invalid transcript %d", i)
		}
		if _, err := invalid.SRT(); err == nil {
			t.Errorf("rendered invalid SRT %d", i)
		}
	}
}
