package microservices_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"
	mediawire "github.com/StephenQiu30/glowloom-server/backend/internal/app/media"
	mediahttp "github.com/StephenQiu30/glowloom-server/backend/media/adapter/http"
	mediaapp "github.com/StephenQiu30/glowloom-server/backend/media/application"
	mediadomain "github.com/StephenQiu30/glowloom-server/backend/media/domain"
)

type actorAssetReader struct {
	actor mediaapp.Actor
	calls int
}

func (r *actorAssetReader) FindAsset(context.Context, mediaapp.Actor, uuid.UUID, uuid.UUID) (mediadomain.MediaAsset, error) {
	return mediadomain.MediaAsset{}, mediaapp.ErrNotFound
}
func (r *actorAssetReader) ListReadyAssets(_ context.Context, actor mediaapp.Actor, _ uuid.UUID, _ string, _ uuid.UUID, _ int) ([]mediadomain.MediaAsset, error) {
	r.actor = actor
	r.calls++
	return nil, nil
}

func TestMediaHTTPActorResolverFailsClosedAndForwardsOnlyServerReference(t *testing.T) {
	for _, mode := range []string{"missing", "malformed", "expired", "valid"} {
		t.Run(mode, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			reader := &actorAssetReader{}
			expected := mediaapp.Actor{ID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New(), SessionID: uuid.New(), CredentialEpoch: 7, PlatformEpoch: 11}
			var resolve mediahttp.ActorResolver
			if mode != "missing" {
				resolve = func(*gin.Context) (mediaapp.Actor, error) {
					if mode == "expired" {
						return mediaapp.Actor{}, mediaapp.ErrSessionInvalid
					}
					if mode == "malformed" {
						return mediaapp.Actor{ID: expected.ID}, nil
					}
					return expected, nil
				}
			}
			router := gin.New()
			mediahttp.NewHandler(mediaapp.NewAssetQuery(reader), resolve).Register(router.Group("/api"))
			request := httptest.NewRequestWithContext(t.Context(), "GET", "/api/projects/"+uuid.NewString()+"/media", nil)
			request.Header.Set("X-Actor-ID", uuid.NewString())
			request.Header.Set("X-Platform-Epoch", "999")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if mode == "valid" {
				if response.Code != 200 || reader.calls != 1 || reader.actor != expected {
					t.Fatalf("server reference changed: status=%d calls=%d actor=%+v", response.Code, reader.calls, reader.actor)
				}
			} else if response.Code != 401 || reader.calls != 0 {
				t.Fatalf("invalid actor reached owner: status=%d calls=%d", response.Code, reader.calls)
			}
		})
	}
}

func TestMediaLegacyActorProjectionPreservesExactEpochWithoutRoleOrGrant(t *testing.T) {
	original := identity.Principal{ID: uuid.New(), OrgID: uuid.New(), ScopeID: uuid.New(), SessionID: uuid.New(), CredentialEpoch: 13, PlatformEpoch: 19, PlatformRole: "platform_admin", LoginName: "synthetic-login", DisplayName: "synthetic-display"}
	actor := mediawire.Actor(original)
	roundtrip := mediawire.IdentityActor(actor)
	if actor.ID != original.ID || actor.OrgID != original.OrgID || actor.ScopeID != original.ScopeID || actor.SessionID != original.SessionID || actor.CredentialEpoch != 13 || actor.PlatformEpoch != 19 || roundtrip.PlatformEpoch != 19 || roundtrip.CredentialEpoch != 13 {
		t.Fatal("authenticated epoch fact changed")
	}
	if roundtrip.PlatformRole != "" || roundtrip.Role != "" || roundtrip.LoginName != "" || roundtrip.DisplayName != "" {
		t.Fatal("authority or profile copied into Actor bridge")
	}
	typ := reflect.TypeFor[mediaapp.Actor]()
	for _, name := range []string{"Role", "PlatformRole", "Grant", "Grants"} {
		if _, ok := typ.FieldByName(name); ok {
			t.Fatalf("Media Actor contains %s", name)
		}
	}
	if err := mediawire.OwningError(identity.ErrSessionInvalid); !errors.Is(err, mediaapp.ErrSessionInvalid) || !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("original session evidence lost")
	}
	if err := mediawire.OwningError(identity.ErrForbidden); !errors.Is(err, mediaapp.ErrForbidden) || !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("original denied authority lost")
	}
}
