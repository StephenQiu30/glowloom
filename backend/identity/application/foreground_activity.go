package application

import (
	"context"
	"crypto/hmac"

	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

// ForegroundPurpose identifies a trusted handler class selected by owner code
// after a successful business response. Public headers and polling never select it.
type ForegroundPurpose string

// These purposes preserve only the existing explicit foreground allowlist.
const (
	ForegroundProfileRead       ForegroundPurpose = "profile_read"
	ForegroundProfileWrite      ForegroundPurpose = "profile_write"
	ForegroundPreferencesRead   ForegroundPurpose = "preferences_read"
	ForegroundPreferencesWrite  ForegroundPurpose = "preferences_write"
	ForegroundSessionsRead      ForegroundPurpose = "sessions_read"
	ForegroundReauthentication  ForegroundPurpose = "reauthentication"
	ForegroundSpacesRead        ForegroundPurpose = "spaces_read"
	ForegroundProjectRead       ForegroundPurpose = "project_read"
	ForegroundProjectWrite      ForegroundPurpose = "project_write"
	ForegroundFolderRead        ForegroundPurpose = "folder_read"
	ForegroundFolderWrite       ForegroundPurpose = "folder_write"
	ForegroundTeamRead          ForegroundPurpose = "team_read"
	ForegroundTeamWrite         ForegroundPurpose = "team_write"
	ForegroundContentWrite      ForegroundPurpose = "content_write"
	ForegroundModelDefaultsRead ForegroundPurpose = "model_defaults_read"
	ForegroundStylePresetsRead  ForegroundPurpose = "style_presets_read"
	ForegroundMediaLibraryRead  ForegroundPurpose = "media_library_read"
	ForegroundModelList         ForegroundPurpose = "model_list"
)

// Owner is the only authenticated service allowed to report this handler class.
// This does not authorize any business effect, resource or write admission.
func (p ForegroundPurpose) Owner() (writefence.Writer, error) {
	switch p {
	case ForegroundProfileRead, ForegroundProfileWrite, ForegroundPreferencesRead, ForegroundPreferencesWrite, ForegroundSessionsRead, ForegroundReauthentication:
		return writefence.WriterIdentity, nil
	case ForegroundSpacesRead, ForegroundProjectRead, ForegroundProjectWrite, ForegroundFolderRead, ForegroundFolderWrite, ForegroundTeamRead, ForegroundTeamWrite, ForegroundContentWrite, ForegroundModelDefaultsRead, ForegroundStylePresetsRead:
		return writefence.WriterCreation, nil
	case ForegroundMediaLibraryRead:
		return writefence.WriterMedia, nil
	case ForegroundModelList:
		return writefence.WriterExecution, nil
	default:
		return "", writefence.ErrInvalid
	}
}
func (p ForegroundPurpose) requiresCSRF() bool {
	switch p {
	case ForegroundProfileWrite, ForegroundPreferencesWrite, ForegroundReauthentication, ForegroundProjectWrite, ForegroundFolderWrite, ForegroundTeamWrite, ForegroundContentWrite:
		return true
	default:
		return false
	}
}

// ForegroundActivityRecorder accepts original credentials, never caller Principals.
type ForegroundActivityRecorder interface {
	RecordForegroundActivity(context.Context, SessionCredential, ForegroundPurpose) (Principal, error)
}

// RecordForegroundActivity rechecks the actual current credential/session before
// extending only its idle activity. Access read purposes may omit CSRF; the public
// CurrentAccess method remains strict and write purposes retain the original proof.
func (a *Auth) RecordForegroundActivity(ctx context.Context, credential SessionCredential, purpose ForegroundPurpose) (Principal, error) {
	if _, err := purpose.Owner(); err != nil {
		return Principal{}, err
	}
	if (credential.LegacyToken == "") == (credential.AccessToken == "") {
		return Principal{}, ErrSessionInvalid
	}
	var result AuthResult
	var err error
	if credential.AccessToken != "" {
		if purpose.requiresCSRF() || credential.CSRFToken != "" {
			result, err = a.CurrentAccess(ctx, credential.AccessToken, credential.CSRFToken, UserActivity)
		} else {
			result, err = a.currentAccess(ctx, credential.AccessToken, "", UserActivity, false)
		}
	} else {
		// Validate a supplied/required proof without activity first; only the second
		// fully locked credential read may perform the activity write.
		result, err = a.CurrentWithActivity(ctx, credential.LegacyToken, NoActivity)
		if err == nil && (purpose.requiresCSRF() || credential.CSRFToken != "") && (credential.CSRFToken == "" || !hmac.Equal([]byte(result.CSRFToken), []byte(credential.CSRFToken))) {
			err = ErrSessionCSRFInvalid
		}
		if err == nil {
			result, err = a.CurrentWithActivity(ctx, credential.LegacyToken, UserActivity)
		}
	}
	if err != nil {
		return Principal{}, err
	}
	return Principal{ID: result.User.ID, SessionID: result.Session.ID, CredentialEpoch: result.User.SessionEpoch}, nil
}
