package domain

import "slices"

// OutlineOptions preserves the seven accepted choices of the source editor.
type OutlineOptions struct {
	ChapterCount   string `json:"chapter_count" binding:"required" enums:"3,5,8,10"`
	Structure      string `json:"structure" binding:"required" enums:"单线推进,双线并行,群像多线,反转嵌套"`
	WordCount      string `json:"word_count" binding:"required" enums:"500,800,1200,2000"`
	Perspective    string `json:"perspective" binding:"required" enums:"第三人称,第一人称,多视角"`
	Tone           string `json:"tone" binding:"required" enums:"平稳叙事,轻松喜剧,紧张悬疑,热血成长,甜宠治愈"`
	CharacterScale string `json:"character_scale" binding:"required" enums:"2 个,3-4 个,5-6 个"`
	ChapterLength  string `json:"chapter_length" binding:"required" enums:"短,中,长"`
}

// Valid rejects implicit defaults and unknown editor options.
func (o OutlineOptions) Valid() bool {
	for _, choice := range []struct {
		value   string
		allowed []string
	}{
		{o.ChapterCount, []string{"3", "5", "8", "10"}},
		{o.Structure, []string{"单线推进", "双线并行", "群像多线", "反转嵌套"}},
		{o.WordCount, []string{"500", "800", "1200", "2000"}},
		{o.Perspective, []string{"第三人称", "第一人称", "多视角"}},
		{o.Tone, []string{"平稳叙事", "轻松喜剧", "紧张悬疑", "热血成长", "甜宠治愈"}},
		{o.CharacterScale, []string{"2 个", "3-4 个", "5-6 个"}},
		{o.ChapterLength, []string{"短", "中", "长"}},
	} {
		if !slices.Contains(choice.allowed, choice.value) {
			return false
		}
	}
	return true
}
