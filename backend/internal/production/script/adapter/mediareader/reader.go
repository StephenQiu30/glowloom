package mediareader

import (
	"context"
	"errors"

	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	scriptapp "github.com/StephenQiu30/lanverse/backend/internal/production/script/application"
)

type Reader struct{ Service *mediaapp.Service }

func (reader Reader) Read(ctx context.Context, actor scriptapp.Actor, versionID string) (scriptapp.MediaContent, []byte, error) {
	version, contents, err := reader.Service.Content(ctx, mediaapp.Actor{UserID: actor.UserID, TokenVersion: actor.TokenVersion}, versionID)
	if err != nil {
		var mediaError *mediaapp.Error
		if errors.As(err, &mediaError) {
			return scriptapp.MediaContent{}, nil, &scriptapp.Error{Code: mediaError.Code, Message: mediaError.Message, Status: mediaError.Status, NextAction: mediaError.NextAction, Details: mediaError.Details}
		}
		return scriptapp.MediaContent{}, nil, err
	}
	return scriptapp.MediaContent{WorkspaceID: version.WorkspaceID, MIMEType: version.MIMEType}, contents, nil
}
