package subject_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/subject/domain"
)

func fact(kind string) domain.MediaFact {
	actor, org := uuid.New(), uuid.New()
	return domain.MediaFact{AssetID: uuid.New(), Revision: 1, Kind: kind, MIMEType: kind + "/test", FileName: "test", SHA256: strings.Repeat("a", 64), ByteSize: 42, PersonalOrgID: &org, PersonalActorID: &actor}
}
func content() domain.Content {
	return domain.Content{SchemaVersion: 1, Name: "主体", Primary: domain.Binding{ID: uuid.New(), Media: fact("image")}, References: []domain.Binding{}}
}
func TestSubjectUsesBrowserNameBudget(t *testing.T) {
	for _, test := range []struct {
		name  string
		valid bool
	}{
		{strings.Repeat("中", 20), true}, {strings.Repeat("中", 21), false},
		{strings.Repeat("😀", 10), true}, {strings.Repeat("😀", 11), false}, {" ", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := content()
			c.Name = test.name
			if got := c.Validate() == nil; got != test.valid {
				t.Fatalf("valid=%v want %v", got, test.valid)
			}
		})
	}
}
func TestSubjectRejectsInvalidReferenceShapes(t *testing.T) {
	cases := []struct {
		name   string
		change func(*domain.Content)
	}{
		{"missing primary", func(c *domain.Content) { c.Primary = domain.Binding{} }},
		{"audio primary", func(c *domain.Content) { c.Primary.Media = fact("audio") }},
		{"video supplement", func(c *domain.Content) { c.References = []domain.Binding{{ID: uuid.New(), Media: fact("video")}} }},
		{"duplicate asset", func(c *domain.Content) { c.References = []domain.Binding{{ID: uuid.New(), Media: c.Primary.Media}} }},
		{"unproved owner", func(c *domain.Content) { c.Primary.Media.PersonalActorID = nil }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			c := content()
			test.change(&c)
			if c.Validate() == nil {
				t.Fatal("accepted invalid subject")
			}
		})
	}
}
