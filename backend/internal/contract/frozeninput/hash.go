// Package frozeninput defines the single technical digest for the private frozen
// input wire request. It owns no policy, business DTO, identity or persistence.
package frozeninput

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationhandoff"
)

// SourceCommandSHA256 binds only the complete immutable wire selection. The
// source digest and returned pin-receipt digests are omitted to avoid recursion;
// the original consumer command and its digest remain intact. Callers map their
// own minimum types to this one generated request, never to another owner's DTO.
func SourceCommandSHA256(request *pb.FreezeFrozenInputRequest) (string, error) {
	if !closed(request) || request.Source == nil || request.Command == nil || request.SourceCommand == nil || proto.Size(request) > 2<<20 {
		return "", errors.New("invalid frozen input wire hash")
	}
	canonical := proto.Clone(request).(*pb.FreezeFrozenInputRequest)
	canonical.SourceCommand.Sha256 = ""
	for _, pin := range canonical.Pins {
		if pin == nil {
			return "", errors.New("invalid frozen input wire pin")
		}
		pin.SnapshotSha256 = ""
	}
	// Source optional UUIDs use absence as their single wire representation. Both
	// an absent ID and an explicit nil UUID decode to the same source selector.
	for _, id := range []*[]byte{&canonical.Source.DraftId, &canonical.Source.CanvasId, &canonical.Source.NodeId} {
		if len(*id) == 16 && bytes.Equal(*id, make([]byte, 16)) {
			*id = nil
		}
	}
	raw, err := (proto.MarshalOptions{Deterministic: true}).Marshal(canonical)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = hash.Write([]byte("glowloom.creation.FreezeFrozenInputRequest\x00"))
	_, _ = hash.Write(raw)
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func closed(value proto.Message) bool {
	if value == nil {
		return false
	}
	message := value.ProtoReflect()
	if !message.IsValid() || len(message.GetUnknown()) != 0 {
		return false
	}
	valid := true
	message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
		if field.Kind() != protoreflect.MessageKind {
			return true
		}
		if field.IsList() {
			list := value.List()
			for i := 0; i < list.Len(); i++ {
				if !closed(list.Get(i).Message().Interface()) {
					valid = false
					break
				}
			}
		} else {
			valid = closed(value.Message().Interface())
		}
		return valid
	})
	return valid
}
