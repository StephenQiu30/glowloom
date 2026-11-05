package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

type historySourceFixture struct{ err error }

func (f historySourceFixture) CheckConversationSources(context.Context, identity.Principal, uuid.UUID, prompt.Draft) error {
	return f.err
}
func TestConversationHistoryHidesRevokedSourceContext(t *testing.T) {
	d, err := prompt.DecodeDraft(conversationJSON(t, true))
	if err != nil {
		t.Fatal(err)
	}
	input, err := prompt.NewConversationInput(d)
	if err != nil {
		t.Fatal(err)
	}
	input.PlainText = "用户的正文"
	input.Prompt = "用户的正文 + 私有描述"
	raw, _ := json.Marshal(AuthorizedConversationInput{ConversationInput: input})
	for _, authority := range []error{errors.New("revoked"), errors.New("offline")} {
		out, err := NewConversationHistory(historySourceFixture{authority}).ReadConversationInput(t.Context(), identity.Principal{}, uuid.New(), raw)
		if err != nil || strings.Contains(string(out), "私有描述") || !strings.Contains(string(out), "references_unavailable") || !strings.Contains(string(out), "用户的正文") {
			t.Fatalf("private source escaped redaction: %s %v", out, err)
		}
	}
	out, err := NewConversationHistory(historySourceFixture{}).ReadConversationInput(t.Context(), identity.Principal{}, uuid.New(), raw)
	if err != nil || string(out) != string(raw) {
		t.Fatal("authorized immutable snapshot changed", err)
	}
}
