package microservices_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func fenceCommand() writefence.Command {
	return writefence.Command{
		ID: uuid.New(), SHA256: strings.Repeat("a", 64), Action: "media.upload.publish",
		Authorities: []writefence.Authority{
			{Issuer: writefence.Identity, Kind: writefence.Session, ID: uuid.New(), Epoch: 1},
			{Issuer: writefence.Creation, Kind: writefence.Project, ID: uuid.New(), Epoch: 4},
		},
	}
}

func TestWriteFenceCommandHasStableAndCompleteAuthorityBinding(t *testing.T) {
	command := fenceCommand()
	original := append([]writefence.Authority(nil), command.Authorities...)
	binding, err := command.AuthorityDigest()
	if err != nil {
		t.Fatal(err)
	}
	command.Authorities[0], command.Authorities[1] = command.Authorities[1], command.Authorities[0]
	reordered, err := command.AuthorityDigest()
	if err != nil || reordered != binding {
		t.Fatalf("authority ordering changed binding: %q, %v", reordered, err)
	}
	if command.Authorities[0] != original[1] {
		t.Fatal("digest mutated caller authority ordering")
	}
	command.Authorities[0].Epoch++
	changed, err := command.AuthorityDigest()
	if err != nil || changed == binding {
		t.Fatalf("epoch omitted from binding: %q, %v", changed, err)
	}
}

func TestWriteFenceRejectsIncompleteOrForgedAuthorityKinds(t *testing.T) {
	tests := map[string]func(*writefence.Command){
		"missing command":        func(c *writefence.Command) { c.ID = uuid.Nil },
		"missing digest":         func(c *writefence.Command) { c.SHA256 = "" },
		"uppercase digest":       func(c *writefence.Command) { c.SHA256 = strings.Repeat("A", 64) },
		"nonhex digest":          func(c *writefence.Command) { c.SHA256 = strings.Repeat("x", 64) },
		"empty action":           func(c *writefence.Command) { c.Action = "" },
		"action control":         func(c *writefence.Command) { c.Action = "publish\n" },
		"unbounded action":       func(c *writefence.Command) { c.Action = strings.Repeat("a", 129) },
		"no authority":           func(c *writefence.Command) { c.Authorities = nil },
		"missing authority id":   func(c *writefence.Command) { c.Authorities[0].ID = uuid.Nil },
		"zero epoch":             func(c *writefence.Command) { c.Authorities[0].Epoch = 0 },
		"negative epoch":         func(c *writefence.Command) { c.Authorities[0].Epoch = -1 },
		"forged issuer":          func(c *writefence.Command) { c.Authorities[0].Issuer = "media" },
		"foreign authority kind": func(c *writefence.Command) { c.Authorities[0].Kind = writefence.Project },
		"duplicate authority":    func(c *writefence.Command) { c.Authorities = append(c.Authorities, c.Authorities[0]) },
		"duplicate with other epoch": func(c *writefence.Command) {
			duplicate := c.Authorities[0]
			duplicate.Epoch++
			c.Authorities = append(c.Authorities, duplicate)
		},
		"unbounded authorities": func(c *writefence.Command) {
			for range 17 {
				c.Authorities = append(c.Authorities, writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: uuid.New(), Epoch: 1})
			}
		},
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			command := fenceCommand()
			change(&command)
			if _, err := command.AuthorityDigest(); !errors.Is(err, writefence.ErrInvalid) {
				t.Fatalf("invalid command accepted: %v", err)
			}
		})
	}
}

func TestWriteFenceReceiptPreservesExactNumbersAndBounds(t *testing.T) {
	raw := json.RawMessage(` {"z":9007199254740993,"nested":{"b":2,"a":1}} `)
	got, err := writefence.CanonicalReceipt(raw)
	if err != nil || string(got) != `{"nested":{"a":1,"b":2},"z":9007199254740993}` {
		t.Fatalf("receipt changed integer precision or did not canonicalize: %s, %v", got, err)
	}
	for name, value := range map[string]string{
		"empty": "", "null": "null", "array": "[]", "multiple": "{}{}", "truncated": "{", "too large": `{"value":"` + strings.Repeat("a", writefence.MaxReceiptBytes) + `"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := writefence.CanonicalReceipt(json.RawMessage(value)); !errors.Is(err, writefence.ErrInvalid) {
				t.Fatalf("invalid receipt accepted: %v", err)
			}
		})
	}
}

func TestWriteFenceControlRequiresExactCoordinationIdentity(t *testing.T) {
	control := writefence.Control{Authority: fenceCommand().Authorities[0], CoordinationID: uuid.New(), Blocked: true}
	if err := control.Validate(); err != nil {
		t.Fatal(err)
	}
	control.CoordinationID = uuid.Nil
	if err := control.Validate(); !errors.Is(err, writefence.ErrInvalid) {
		t.Fatalf("control without durable coordination accepted: %v", err)
	}
}
