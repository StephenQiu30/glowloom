package domain

import (
	"crypto/sha256"
	"fmt"

	"github.com/google/uuid"
)

// Published content keeps its identity across a product rename. New baseline
// content uses the current namespace; stored user preferences are never rebased.
var publishedPromptBaselines = map[string]struct{ digest, id string }{
	"chapter_assets_extract": {"89c7bf830bc1463cf5b09a6e3c54763068759a03614cd35be0190896338b62d3", "ceb2eb5d-bbed-57e1-b3f5-556205fcb8af"},
	"storyboard_plan":        {"0a7186c6a7438c9ffb4d742e68ccff333dbd5a179769767425eb29245e3d23ef", "7980a0e2-9872-55a4-ba4a-f7b76eee3af0"},
	"storyboard_repair":      {"06b09a27a845089b08f5971a11651ea1d935e27e280d377d7abe3a2de827e6df", "bb86b654-2ea4-5b5f-8fd2-d88a6b32b071"},
	"storyboard_first_frame": {"fee2de75cdb2f9ae593a680c9f5d25f53ef808331366094c8cd38ed99b588173", "69eea5a0-4c56-5f86-97a7-c607d34d3fe3"},
	"storyboard_video":       {"fd5952d1ee75b765456f4c4b0a0ae9c868599226809506587216b08829cfd329", "edbaf5ed-84e4-5527-a8fb-008c8b540b47"},
	"character_extract":      {"3b81f4a23f6c93cbece03c10cbb55ce3a2a993e71bb23f6935aa1e097690af15", "eae3f8e7-280a-5d9a-a030-23534f1d74ca"},
	"character_turnaround":   {"828fbfd66df462eb074184d705e898a3a19f8a6b0445d6ef8d61b5d74baa48bc", "fd7f7f3d-258e-5a6b-85c3-49d6265c6f10"},
	"short_drama_outline":    {"1c524db159f73794a9e7fbfa40d8895cc5b2278d22303decdaebadd742067ff7", "5518a988-e170-58aa-b14b-3d147226dcba"},
	"skill_draft":            {"7f13a1bb2003938e9695dc0c44b069211adef8d2b5c9c90bd9beeae77fba3715", "7b335bb8-461b-5cef-ba14-f1a75826cd0e"},
}

func promptTemplateID(operation string, digest [sha256.Size]byte) uuid.UUID {
	if published, ok := publishedPromptBaselines[operation]; ok && published.digest == fmt.Sprintf("%x", digest) {
		return uuid.MustParse(published.id)
	}
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte(fmt.Sprintf("glowloom/prompt/%s/%x", operation, digest)))
}
