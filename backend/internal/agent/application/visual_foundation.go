package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/StephenQiu30/lanverse/backend/internal/agent/contract"
)

type VisualFoundationMedia struct {
	AttachmentID string
	MediaType    string
	Contents     []byte
}

type VisualFoundationObjectReader interface {
	ReadVerified(context.Context, string, int64, string, int64) ([]byte, error)
}

type VisualFoundationRuntime interface {
	InvokeVisualFoundation(
		context.Context,
		contract.VisualFoundationInvocation,
		contract.SceneAnalysisDispatchAuthorization,
		[]VisualFoundationMedia,
	) (contract.VisualFoundationAttemptResult, error)
}

type VisualFoundationMediaBroker struct {
	objects VisualFoundationObjectReader
	runtime VisualFoundationRuntime
}

func NewVisualFoundationMediaBroker(
	objects VisualFoundationObjectReader,
	runtime VisualFoundationRuntime,
) (*VisualFoundationMediaBroker, error) {
	if objects == nil || runtime == nil {
		return nil, errors.New("Visual Foundation media broker dependencies are required")
	}
	return &VisualFoundationMediaBroker{objects: objects, runtime: runtime}, nil
}

func (broker *VisualFoundationMediaBroker) Invoke(
	ctx context.Context,
	invocation contract.VisualFoundationInvocation,
	authorization contract.SceneAnalysisDispatchAuthorization,
) (contract.VisualFoundationAttemptResult, error) {
	if err := invocation.Validate(); err != nil {
		return contract.VisualFoundationAttemptResult{}, err
	}
	if err := authorization.Validate(); err != nil {
		return contract.VisualFoundationAttemptResult{}, err
	}
	media := make([]VisualFoundationMedia, 0, len(invocation.Payload.MediaAttachments))
	for _, attachment := range invocation.Payload.MediaAttachments {
		contents, err := broker.objects.ReadVerified(
			ctx,
			attachment.ObjectKey,
			attachment.ByteLength,
			attachment.ContentHash,
			contract.MaxVisualFoundationImageBytes,
		)
		if err != nil {
			return contract.VisualFoundationAttemptResult{}, err
		}
		digest := sha256.Sum256(contents)
		if int64(len(contents)) != attachment.ByteLength ||
			hex.EncodeToString(digest[:]) != attachment.ContentHash {
			return contract.VisualFoundationAttemptResult{}, &Error{
				Code:    "media_attachment_invalid",
				Message: "Visual Foundation media content drifted after object read",
			}
		}
		media = append(media, VisualFoundationMedia{
			AttachmentID: attachment.AttachmentID,
			MediaType:    attachment.MediaType,
			Contents:     contents,
		})
	}
	return broker.runtime.InvokeVisualFoundation(ctx, invocation, authorization, media)
}
