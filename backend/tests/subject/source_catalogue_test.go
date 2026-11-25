package subject_test

import (
	"testing"

	media "github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
)

func TestSubjectCatalogueQuerySupportsRealUpscaleWithoutOwnerOverrides(t *testing.T) {
	valid := media.SubjectCatalogueQuery{Kind: "image", Quality: "upscaled", Page: 1, PageSize: 12}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*media.SubjectCatalogueQuery){func(q *media.SubjectCatalogueQuery) { q.Kind = "text" }, func(q *media.SubjectCatalogueQuery) { q.Page = 0 }, func(q *media.SubjectCatalogueQuery) { q.PageSize = 121 }, func(q *media.SubjectCatalogueQuery) { q.Quality = "4k" }, func(q *media.SubjectCatalogueQuery) { q.Kind = "audio" }} {
		input := valid
		change(&input)
		if input.Validate() == nil {
			t.Fatal("invalid source query accepted", input)
		}
	}
}
