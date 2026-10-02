package script_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
)

func manualStructure() domain.StructureDocument {
	return domain.StructureDocument{Scenes: []domain.StructureScene{{Key: uuid.New(), SeqNo: 1, Heading: "客厅 日 内", LocationText: "客厅", TimeOfDay: "日", Start: 0, End: 9, Items: []domain.StructureItem{{Type: "action", Key: uuid.New(), Content: "走进客厅", Start: 0, End: 4}, {Type: "line", Key: uuid.New(), Kind: "dialogue", Speaker: "小明", Content: "你好😀", Emotion: "惊讶", Start: 5, End: 8}}}}, Unassigned: []domain.UnassignedLine{}}
}
func TestScriptManualStructureClosedOrderSpansAndStableIdentity(t *testing.T) {
	doc := manualStructure()
	raw, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := domain.DecodeStructure(raw, 0, 9)
	if err != nil || parsed.Scenes[0].Items[0].Type != "action" || parsed.Scenes[0].Items[1].Type != "line" || parsed.Scenes[0].Items[1].Emotion != "惊讶" || parsed.Scenes[0].Key != doc.Scenes[0].Key {
		t.Fatal("ordered editable structure", parsed, err)
	}
	for _, mutate := range []func(*domain.StructureDocument){func(d *domain.StructureDocument) { d.Scenes[0].Items[1].Key = d.Scenes[0].Items[0].Key }, func(d *domain.StructureDocument) { d.Scenes[0].Items[1].Start = 2 }, func(d *domain.StructureDocument) { d.Scenes[0].Items[1].End = 10 }, func(d *domain.StructureDocument) { d.Scenes[0].Items[0].Speaker = "伪动作角色" }, func(d *domain.StructureDocument) { d.Scenes[0].Items[1].Kind = "unsupported" }, func(d *domain.StructureDocument) { d.Scenes[0].SeqNo = 0 }} {
		d := manualStructure()
		mutate(&d)
		if err := d.Validate(0, 9); err == nil {
			t.Fatal("invalid structure accepted", d)
		}
	}
}
func TestScriptManualStructureRejectsUnknownAndUnassignedOverlap(t *testing.T) {
	if _, err := domain.DecodeStructure([]byte(`{"scenes":[],"unassigned_lines":[],"hidden_lines":[]}`), 0, 9); err == nil {
		t.Fatal("hidden field accepted")
	}
	doc := manualStructure()
	doc.Unassigned = append(doc.Unassigned, domain.UnassignedLine{Key: uuid.New(), Content: "重复正文", Start: 5, End: 8})
	if err := doc.Validate(0, 9); err == nil {
		t.Fatal("unassigned duplicates assigned line")
	}
}
