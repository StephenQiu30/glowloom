package grpctransport

import (
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// EncodeCommand reuses the closed command envelope in typed owner protocols.
func EncodeCommand(command writefence.Command) *pb.Command { return encodeCommand(command) }

// DecodeCommand validates issuer/kind/ID/epoch and the exact bounded authority set.
func DecodeCommand(value *pb.Command) (writefence.Command, error) { return decodeCommand(value) }
