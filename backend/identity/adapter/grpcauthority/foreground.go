package grpcauthority

import (
	"context"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/identity"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

var foregroundPurposes = map[identity.ForegroundPurpose]pb.ForegroundPurpose{
	identity.ForegroundProfileRead:       pb.ForegroundPurpose_FOREGROUND_PURPOSE_PROFILE_READ,
	identity.ForegroundProfileWrite:      pb.ForegroundPurpose_FOREGROUND_PURPOSE_PROFILE_WRITE,
	identity.ForegroundPreferencesRead:   pb.ForegroundPurpose_FOREGROUND_PURPOSE_PREFERENCES_READ,
	identity.ForegroundPreferencesWrite:  pb.ForegroundPurpose_FOREGROUND_PURPOSE_PREFERENCES_WRITE,
	identity.ForegroundSessionsRead:      pb.ForegroundPurpose_FOREGROUND_PURPOSE_SESSIONS_READ,
	identity.ForegroundReauthentication:  pb.ForegroundPurpose_FOREGROUND_PURPOSE_REAUTHENTICATION,
	identity.ForegroundSpacesRead:        pb.ForegroundPurpose_FOREGROUND_PURPOSE_SPACES_READ,
	identity.ForegroundProjectRead:       pb.ForegroundPurpose_FOREGROUND_PURPOSE_PROJECT_READ,
	identity.ForegroundProjectWrite:      pb.ForegroundPurpose_FOREGROUND_PURPOSE_PROJECT_WRITE,
	identity.ForegroundFolderRead:        pb.ForegroundPurpose_FOREGROUND_PURPOSE_FOLDER_READ,
	identity.ForegroundFolderWrite:       pb.ForegroundPurpose_FOREGROUND_PURPOSE_FOLDER_WRITE,
	identity.ForegroundTeamRead:          pb.ForegroundPurpose_FOREGROUND_PURPOSE_TEAM_READ,
	identity.ForegroundTeamWrite:         pb.ForegroundPurpose_FOREGROUND_PURPOSE_TEAM_WRITE,
	identity.ForegroundContentWrite:      pb.ForegroundPurpose_FOREGROUND_PURPOSE_CONTENT_WRITE,
	identity.ForegroundModelDefaultsRead: pb.ForegroundPurpose_FOREGROUND_PURPOSE_MODEL_DEFAULTS_READ,
	identity.ForegroundStylePresetsRead:  pb.ForegroundPurpose_FOREGROUND_PURPOSE_STYLE_PRESETS_READ,
	identity.ForegroundMediaLibraryRead:  pb.ForegroundPurpose_FOREGROUND_PURPOSE_MEDIA_LIBRARY_READ,
	identity.ForegroundModelList:         pb.ForegroundPurpose_FOREGROUND_PURPOSE_MODEL_LIST,
}

func decodeForegroundPurpose(value pb.ForegroundPurpose) (identity.ForegroundPurpose, error) {
	for purpose, wire := range foregroundPurposes {
		if value == wire {
			return purpose, nil
		}
	}
	return "", writefence.ErrInvalid
}

func (s *server) RecordForegroundActivity(ctx context.Context, request *pb.RecordForegroundActivityRequest) (*pb.RecordForegroundActivityResponse, error) {
	purpose, err := decodeForegroundPurpose(request.Purpose)
	if err != nil {
		return nil, rpcError(err)
	}
	credential, err := decodeCredential(request.Credential)
	if err != nil {
		return nil, rpcError(err)
	}
	recorder, ok := s.auth.(identity.ForegroundActivityRecorder)
	if !ok {
		return nil, rpcError(writefence.ErrUnavailable)
	}
	actor, err := recorder.RecordForegroundActivity(ctx, identity.SessionCredential{LegacyToken: credential.legacy, AccessToken: credential.access, CSRFToken: credential.csrf}, purpose)
	if err != nil {
		return nil, rpcError(err)
	}
	id := Identity{AccountID: actor.ID, SessionID: actor.SessionID, CredentialEpoch: actor.CredentialEpoch}
	if _, err := decodeIdentity(encodeIdentity(id)); err != nil {
		return nil, rpcError(writefence.ErrUnavailable)
	}
	return &pb.RecordForegroundActivityResponse{Identity: encodeIdentity(id)}, nil
}

// RecordForegroundActivity explicitly reports a successful trusted handler class.
// Identity owns this adapter; other owners consume the generated protobuf contract
// in their own adapters, without importing this server/application dependency.
func (c *Client) RecordForegroundActivity(ctx context.Context, credential Credential, purpose identity.ForegroundPurpose) (Identity, error) {
	owner, err := purpose.Owner()
	if err != nil || c == nil || owner != c.caller {
		return Identity{}, writefence.ErrInvalid
	}
	if !validCredential(credential) {
		return Identity{}, writefence.ErrInvalid
	}
	ctx, cancel, conn, err := c.connect(ctx)
	if err != nil {
		return Identity{}, err
	}
	defer cancel()
	defer func() { _ = conn.Close() }()
	response, err := pb.NewAuthorityClient(conn).RecordForegroundActivity(ctx, &pb.RecordForegroundActivityRequest{Caller: encodeWriter(c.caller), Credential: encodeCredential(credential), Purpose: foregroundPurposes[purpose]})
	if err != nil {
		return Identity{}, localError(err)
	}
	if !closedMessage(response) {
		return Identity{}, writefence.ErrUnavailable
	}
	return decodeIdentity(response.Identity)
}
