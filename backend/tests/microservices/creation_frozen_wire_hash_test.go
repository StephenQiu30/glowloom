package microservices_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	prompt "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	wirehash "github.com/StephenQiu30/glowloom-server/backend/internal/contract/frozeninput"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationhandoff"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

func frozenWireHashFixture() (prompt.FreezeInputRequest, *pb.FreezeFrozenInputRequest) {
	identity, admission := sourceAdmissionFixture()
	admission.Command.Action = "execution.frozen_input.accept"
	request := prompt.FreezeInputRequest{HandoffID: uuid.New(), RequestID: uuid.New(), Command: admission.Command, ActorID: identity.AccountID, SessionID: identity.SessionID, ScopeID: admission.ScopeID, ProjectID: admission.ProjectID, Source: domain.InputSource{Kind: "prompt_draft", DraftID: uuid.New(), Revision: 3}, SourceGeneration: 2, ExpectedScopeRevision: 4, ExpectedProjectRevision: 5, Pins: []domain.FrozenPin{{ReferenceID: "image:1", PinID: uuid.New(), AcquireCommandID: uuid.New(), AssetID: uuid.New(), AssetRevision: 2, Kind: "image", SnapshotSHA256: strings.Repeat("a", 64)}}}
	request.SourceCommand = writefence.Command{ID: uuid.NewSHA1(request.HandoffID, []byte("creation.frozen_input.freeze")), Action: "creation.frozen_input.freeze", Authorities: append([]writefence.Authority(nil), request.Command.Authorities...)}
	wire := sourceFreezeWire(request)
	pin := request.Pins[0]
	wire.Pins = []*pb.FrozenInputPin{{ReferenceId: pin.ReferenceID, PinId: pin.PinID[:], AcquireCommandId: pin.AcquireCommandID[:], AssetId: pin.AssetID[:], AssetRevision: pin.AssetRevision, Kind: pin.Kind, SnapshotSha256: pin.SnapshotSHA256}}
	return request, wire
}

func TestFrozenInputSourceWireHashMatchesCreationWithoutMutatingWire(t *testing.T) {
	request, wire := frozenWireHashFixture()
	before := proto.Clone(wire)
	creation, err := request.SourceCommandSHA256()
	if err != nil {
		t.Fatal(err)
	}
	consumer, err := wirehash.SourceCommandSHA256(wire)
	if err != nil || consumer != creation || len(consumer) != 64 {
		t.Fatalf("owner-independent wire digest drift: %q/%q %v", creation, consumer, err)
	}
	if !proto.Equal(before, wire) {
		t.Fatal("hash changed caller-owned credentials/selection data")
	}
	wire.SourceCommand.Sha256 = strings.Repeat("b", 64)
	wire.Pins[0].SnapshotSha256 = strings.Repeat("c", 64)
	wire.Source.CanvasId = make([]byte, 16)
	wire.Source.NodeId = make([]byte, 16)
	stable, err := wirehash.SourceCommandSHA256(wire)
	if err != nil || stable != consumer {
		t.Fatalf("recursive receipt fields or nil UUID representation changed the source digest: %v", err)
	}
}

func TestFrozenInputSourceWireHashBindsCompleteTemplateSelection(t *testing.T) {
	request, wire := frozenWireHashFixture()
	definition, _ := domain.DefinitionFor("short_drama_outline")
	revision := int64(0)
	request.TemplateSelection = &prompt.TemplateRequest{Operation: definition.Operation, ExpectedTemplateID: definition.TemplateID, ExpectedCustomizationRevision: &revision, Outline: &prompt.OutlineOptions{ChapterCount: "3", Structure: "单线推进", WordCount: "500", Perspective: "第三人称", Tone: "平稳叙事", CharacterScale: "2 个", ChapterLength: "短"}}
	wire.TemplateSelection = sourceFreezeWire(request).TemplateSelection
	creation, err := request.SourceCommandSHA256()
	if err != nil {
		t.Fatal(err)
	}
	baseline, err := wirehash.SourceCommandSHA256(wire)
	if err != nil || baseline != creation {
		t.Fatal("template selection mapping diverged from unique protobuf digest", err)
	}
	for _, field := range []string{"missing-selection", "operation", "template-id", "revision", "revision-presence", "chapter-count", "structure", "word-count", "perspective", "tone", "character-scale", "chapter-length"} {
		t.Run(field, func(t *testing.T) {
			changed := proto.Clone(wire).(*pb.FreezeFrozenInputRequest)
			selection := changed.TemplateSelection
			switch field {
			case "missing-selection":
				changed.TemplateSelection = nil
			case "operation":
				selection.Operation = "skill_draft"
			case "template-id":
				id := uuid.New()
				selection.ExpectedTemplateId = id[:]
			case "revision":
				value := int64(1)
				selection.ExpectedCustomizationRevision = &value
			case "revision-presence":
				selection.ExpectedCustomizationRevision = nil
			case "chapter-count":
				selection.Outline.ChapterCount = "5"
			case "structure":
				selection.Outline.Structure = "双线并行"
			case "word-count":
				selection.Outline.WordCount = "800"
			case "perspective":
				selection.Outline.Perspective = "第一人称"
			case "tone":
				selection.Outline.Tone = "轻松喜剧"
			case "character-scale":
				selection.Outline.CharacterScale = "3-4 个"
			case "chapter-length":
				selection.Outline.ChapterLength = "中"
			}
			hash, err := wirehash.SourceCommandSHA256(changed)
			if err != nil || hash == baseline {
				t.Fatal("template selection field escaped original source command digest", field, err)
			}
		})
	}
	for _, outline := range []bool{false, true} {
		changed := proto.Clone(wire).(*pb.FreezeFrozenInputRequest)
		var message proto.Message = changed.TemplateSelection
		if outline {
			message = changed.TemplateSelection.Outline
		}
		message.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
		if _, err := wirehash.SourceCommandSHA256(changed); err == nil {
			t.Fatal("unknown template selection field accepted")
		}
	}
}

func TestFrozenInputSourceWireHashBindsOriginalIntentSelectionAndPinIdentity(t *testing.T) {
	_, original := frozenWireHashFixture()
	baseline, err := wirehash.SourceCommandSHA256(original)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"consumer-sha", "consumer-id", "consumer-action", "consumer-epoch", "source-id", "source-epoch", "handoff", "request", "scope", "project", "source-version", "source-generation", "scope-version", "project-version", "pin", "pin-acquisition", "asset", "asset-version", "reference"} {
		t.Run(field, func(t *testing.T) {
			changed := proto.Clone(original).(*pb.FreezeFrozenInputRequest)
			id := uuid.New()
			switch field {
			case "consumer-sha":
				changed.Command.Sha256 = strings.Repeat("b", 64)
			case "consumer-id":
				changed.Command.Id = id[:]
			case "consumer-action":
				changed.Command.Action = "other.accept"
			case "consumer-epoch":
				changed.Command.Authorities[0].Epoch++
			case "source-id":
				changed.SourceCommand.Id = id[:]
			case "source-epoch":
				changed.SourceCommand.Authorities[0].Epoch++
			case "handoff":
				changed.HandoffId = id[:]
			case "request":
				changed.RequestId = id[:]
			case "scope":
				changed.ScopeId = id[:]
			case "project":
				changed.ProjectId = id[:]
			case "source-version":
				changed.Source.Revision++
			case "source-generation":
				changed.SourceGeneration++
			case "scope-version":
				changed.ExpectedScopeRevision++
			case "project-version":
				changed.ExpectedProjectRevision++
			case "pin":
				changed.Pins[0].PinId = id[:]
			case "pin-acquisition":
				changed.Pins[0].AcquireCommandId = id[:]
			case "asset":
				changed.Pins[0].AssetId = id[:]
			case "asset-version":
				changed.Pins[0].AssetRevision++
			case "reference":
				changed.Pins[0].ReferenceId = "image:2"
			}
			hash, err := wirehash.SourceCommandSHA256(changed)
			if err != nil || hash == baseline {
				t.Fatalf("wire digest failed to bind %s: %v", field, err)
			}
		})
	}
	for _, part := range []string{"envelope", "command", "authority", "source", "pin"} {
		t.Run("unknown-"+part, func(t *testing.T) {
			changed := proto.Clone(original).(*pb.FreezeFrozenInputRequest)
			var message proto.Message = changed
			switch part {
			case "command":
				message = changed.Command
			case "authority":
				message = changed.Command.Authorities[0]
			case "source":
				message = changed.Source
			case "pin":
				message = changed.Pins[0]
			}
			message.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
			if _, err := wirehash.SourceCommandSHA256(changed); err == nil {
				t.Fatalf("unknown %s entered the canonical hash", part)
			}
		})
	}
}

func TestCreationFrozenSnapshotUsesActualLockedProjectAspectRatio(t *testing.T) {
	for _, aspect := range []string{"9:16", "16:9", "1:1"} {
		t.Run(aspect, func(t *testing.T) {
			f := newFrozenFixture(t, aspect)
			first := f.freeze(t)
			if first.Input.AspectRatio != aspect {
				t.Fatalf("project ratio replaced by a default: %q want %q", first.Input.AspectRatio, aspect)
			}
			if err := f.admin.Exec(`UPDATE workspace.project SET description='edited metadata',revision=revision+1 WHERE id=?`, f.request.ProjectID).Error; err != nil {
				t.Fatal(err)
			}
			replay := f.freeze(t)
			if replay.Input.AspectRatio != aspect || replay.SnapshotSHA256 != first.SnapshotSHA256 {
				t.Fatal("original frozen settings changed after a mutable project edit")
			}
		})
	}
}
