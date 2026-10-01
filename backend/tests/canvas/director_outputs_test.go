package canvas_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/canvas/domain"
)

func directorScreenshot() domain.DirectorScreenshot {
	return domain.DirectorScreenshot{ID: uuid.New(), AssetID: uuid.New(), Name: "机位截图 1", CreatedAt: time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC)}
}

func TestDirectorOutputsRoundTripAndValidateClosedReferences(t *testing.T) {
	node := directorNode()
	screenshot := directorScreenshot()
	node.Config.Director.Shots[0].Screenshots = []domain.DirectorScreenshot{screenshot}
	node.Config.Director.Cover = &domain.DirectorCover{AssetID: screenshot.AssetID, ShotID: node.Config.Director.Shots[0].ID}
	raw, err := json.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}
	var decoded domain.Node
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	result, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{decoded}}})
	if err != nil || len(result.Nodes) != 1 {
		t.Fatalf("valid outputs rejected: %v", err)
	}
	if got := result.Nodes[0].Config.Director; got.Cover.AssetID != screenshot.AssetID || len(got.Shots[0].Screenshots) != 1 || got.Shots[0].Screenshots[0] != screenshot {
		t.Fatalf("outputs changed during round trip: %+v", got)
	}
	for _, tc := range []struct {
		name   string
		change func(*domain.DirectorConfig)
	}{
		{"missing screenshot identity", func(c *domain.DirectorConfig) { c.Shots[0].Screenshots[0].ID = uuid.Nil }},
		{"missing screenshot asset", func(c *domain.DirectorConfig) { c.Shots[0].Screenshots[0].AssetID = uuid.Nil }},
		{"empty name", func(c *domain.DirectorConfig) { c.Shots[0].Screenshots[0].Name = "" }},
		{"blank name", func(c *domain.DirectorConfig) { c.Shots[0].Screenshots[0].Name = " \n" }},
		{"long name", func(c *domain.DirectorConfig) { c.Shots[0].Screenshots[0].Name = strings.Repeat("图", 129) }},
		{"NUL name", func(c *domain.DirectorConfig) { c.Shots[0].Screenshots[0].Name = "图\x00" }},
		{"invalid UTF8 name", func(c *domain.DirectorConfig) { c.Shots[0].Screenshots[0].Name = string([]byte{0xff}) }},
		{"missing capture time", func(c *domain.DirectorConfig) { c.Shots[0].Screenshots[0].CreatedAt = time.Time{} }},
		{"time outside RFC3339", func(c *domain.DirectorConfig) {
			c.Shots[0].Screenshots[0].CreatedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
		}},
		{"missing cover asset", func(c *domain.DirectorConfig) { c.Cover.AssetID = uuid.Nil }},
		{"missing cover shot", func(c *domain.DirectorConfig) { c.Cover.ShotID = uuid.New() }},
		{"duplicate screenshot within shot", func(c *domain.DirectorConfig) {
			c.Shots[0].Screenshots = append(c.Shots[0].Screenshots, c.Shots[0].Screenshots[0])
		}},
		{"duplicate screenshot across shots", func(c *domain.DirectorConfig) {
			shot := c.Shots[0]
			shot.ID = uuid.New()
			c.Shots = append(c.Shots, shot)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			n := directorNode()
			n.Config.Director.Shots[0].Screenshots = []domain.DirectorScreenshot{directorScreenshot()}
			n.Config.Director.Cover = &domain.DirectorCover{AssetID: uuid.New(), ShotID: n.Config.Director.Shots[0].ID}
			tc.change(n.Config.Director)
			_, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{n}}})
			if !errors.Is(err, domain.ErrInvalidCommand) {
				t.Fatalf("invalid output accepted: %v", err)
			}
		})
	}
}

func TestDirectorScreenshotLimitsAndNames(t *testing.T) {
	for _, tc := range []struct {
		name     string
		shots    int
		perShot  int
		lastShot int
		valid    bool
	}{
		{"per shot boundary", 1, 64, 64, true},
		{"per shot overflow", 1, 65, 65, false},
		{"scene boundary", 8, 64, 64, true},
		{"scene overflow", 9, 64, 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			node := directorNode()
			c := node.Config.Director
			shot := c.Shots[0]
			c.Shots = nil
			for i := range tc.shots {
				copyShot := shot
				copyShot.ID = uuid.New()
				count := tc.perShot
				if i == tc.shots-1 {
					count = tc.lastShot
				}
				for range count {
					screenshot := directorScreenshot()
					screenshot.Name = strings.Repeat("图", 128)
					copyShot.Screenshots = append(copyShot.Screenshots, screenshot)
				}
				c.Shots = append(c.Shots, copyShot)
			}
			c.ActiveShotID = c.Shots[0].ID
			_, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}})
			if (err == nil) != tc.valid {
				t.Fatalf("expected valid=%v: %v", tc.valid, err)
			}
		})
	}
}

func TestDirectorDeletingShotRequiresRemovingCover(t *testing.T) {
	node := directorNode()
	node.Config.Director.Cover = &domain.DirectorCover{AssetID: uuid.New(), ShotID: node.Config.Director.Shots[0].ID}
	second := node.Config.Director.Shots[0]
	second.ID = uuid.New()
	node.Config.Director.Shots = append(node.Config.Director.Shots, second)
	doc, err := domain.Apply(domain.Document{}, []domain.Command{{Type: "AddNodes", Nodes: []domain.Node{node}}})
	if err != nil {
		t.Fatal(err)
	}
	config := *node.Config.Director
	config.Shots = []domain.DirectorShot{second}
	config.ActiveShotID = second.ID
	if _, err := domain.Apply(doc, []domain.Command{{Type: "UpdateNodeConfig", ID: node.ID, Config: &domain.NodeConfig{Director: &config}}}); !errors.Is(err, domain.ErrInvalidCommand) {
		t.Fatalf("dangling cover survived shot deletion: %v", err)
	}
	config.Cover = nil
	if _, err := domain.Apply(doc, []domain.Command{{Type: "UpdateNodeConfig", ID: node.ID, Config: &domain.NodeConfig{Director: &config}}}); err != nil {
		t.Fatalf("valid shot removal rejected: %v", err)
	}
	if doc.Nodes[0].Config.Director.Cover == nil || len(doc.Nodes[0].Config.Director.Shots) != 2 {
		t.Fatal("validation mutated original scene")
	}
}
