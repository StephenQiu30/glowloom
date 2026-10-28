package canvas_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/canvas/application"
)

func TestCanvasAssetCommandRejectsAmbiguousOrInvalidMutations(t *testing.T) {
	id := uuid.New()
	favorite := true
	for _, input := range []application.AssetCommandInput{
		{Action: "publish", Items: []application.AssetCommandItem{{ID: id}}},
		{Action: "favorite", Items: []application.AssetCommandItem{{ID: id}}},
		{Action: "favorite", Favorite: &favorite, Items: []application.AssetCommandItem{{ID: id}, {ID: id}}},
		{Action: "delete", Favorite: &favorite, Items: []application.AssetCommandItem{{ID: id, ExpectedRevision: 1}}},
		{Action: "delete", Items: []application.AssetCommandItem{{ID: id}}},
		{Action: "rename", Name: "新名称", Items: []application.AssetCommandItem{{ID: id, ExpectedRevision: 1}, {ID: uuid.New(), ExpectedRevision: 1}}},
	} {
		if input.Validate() == nil {
			t.Fatalf("invalid command accepted: %+v", input)
		}
	}
	for _, input := range []application.AssetCommandInput{
		{Action: "favorite", Favorite: &favorite, Items: []application.AssetCommandItem{{ID: id}}},
		{Action: "delete", Items: []application.AssetCommandItem{{ID: id, ExpectedRevision: 1}}},
		{Action: "rename", Name: "新名称", Items: []application.AssetCommandItem{{ID: id, ExpectedRevision: 1}}},
	} {
		if err := input.Validate(); err != nil {
			t.Fatal(input, err)
		}
	}
}
