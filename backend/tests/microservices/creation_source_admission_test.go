package microservices_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"
)

type creationIdentityAuthority struct {
	identity            workspacesource.Identity
	verifyErr, admitErr error
	calls               *[]string
}

func (a creationIdentityAuthority) VerifySourceIdentity(context.Context, workspacesource.Credential) (workspacesource.Identity, error) {
	*a.calls = append(*a.calls, "verify")
	return a.identity, a.verifyErr
}
func (a creationIdentityAuthority) AdmitSourceSession(context.Context, workspacesource.Credential, writefence.Command, writefence.Writer) (workspacesource.Identity, error) {
	*a.calls = append(*a.calls, "identity-admit")
	return a.identity, a.admitErr
}

type creationSourceStore struct {
	calls *[]string
	err   error
}

func (s creationSourceStore) RegisterSourceCommand(_ context.Context, id workspacesource.Identity, request workspacesource.AdmissionRequest) (workspacesource.Permit, error) {
	*s.calls = append(*s.calls, "creation-admit")
	return workspacesource.Permit{AccountID: id.AccountID, SessionID: id.SessionID, ScopeID: request.ScopeID, ProjectID: request.ProjectID, PhysicalScopeID: uuid.New(), Command: request.Command, Target: request.Target}, s.err
}

func sourceAdmissionFixture() (workspacesource.Identity, workspacesource.AdmissionRequest) {
	id := workspacesource.Identity{AccountID: uuid.New(), SessionID: uuid.New(), CredentialEpoch: 3}
	scope, project := uuid.New(), uuid.New()
	return id, workspacesource.AdmissionRequest{ScopeID: scope, ProjectID: project, Target: writefence.WriterCreation, Command: writefence.Command{ID: uuid.New(), SHA256: strings.Repeat("a", 64), Action: "creation.prompt.edit", Authorities: []writefence.Authority{
		{Issuer: writefence.Identity, Kind: writefence.Account, ID: id.AccountID, Epoch: 2},
		{Issuer: writefence.Identity, Kind: writefence.Session, ID: id.SessionID, Epoch: 4},
		{Issuer: writefence.Creation, Kind: writefence.Scope, ID: scope, Epoch: 1},
		{Issuer: writefence.Creation, Kind: writefence.Project, ID: project, Epoch: 1},
	}}}
}

func TestCreationSourceAdmissionRequiresIdentityCommandAdmissionBeforeLocalPolicy(t *testing.T) {
	id, request := sourceAdmissionFixture()
	for _, fail := range []string{"", "verify", "identity-admit", "creation-admit"} {
		t.Run(fail, func(t *testing.T) {
			var calls []string
			a := creationIdentityAuthority{identity: id, calls: &calls}
			s := creationSourceStore{calls: &calls}
			failure := errors.New("dependency unavailable")
			switch fail {
			case "verify":
				a.verifyErr = failure
			case "identity-admit":
				a.admitErr = failure
			case "creation-admit":
				s.err = failure
			}
			service := workspacesource.NewAdmission(a, s)
			permit, err := service.Admit(t.Context(), workspacesource.LegacyCredential("original", "csrf"), request)
			if fail == "" {
				if err != nil || permit.Command.ID != request.Command.ID {
					t.Fatalf("permit: %+v %v", permit, err)
				}
			} else if !errors.Is(err, failure) {
				t.Fatalf("failure lost: %v", err)
			}
			want := "verify,identity-admit,creation-admit"
			if fail == "verify" {
				want = "verify"
			}
			if fail == "identity-admit" {
				want = "verify,identity-admit"
			}
			if strings.Join(calls, ",") != want {
				t.Fatalf("calls = %v; want %s", calls, want)
			}
		})
	}
}

func TestCreationSourceAdmissionRejectsUnboundIdentityAndAuthorityBeforeAdmission(t *testing.T) {
	for _, shape := range []string{"foreign-account", "foreign-session", "extra-account", "extra-scope", "missing-scope", "foreign-project", "agent-grant", "unknown-action", "wrong-owner"} {
		t.Run(shape, func(t *testing.T) {
			id, request := sourceAdmissionFixture()
			switch shape {
			case "foreign-account":
				request.Command.Authorities[0].ID = uuid.New()
			case "foreign-session":
				request.Command.Authorities[1].ID = uuid.New()
			case "extra-account":
				request.Command.Authorities = append(request.Command.Authorities, writefence.Authority{Issuer: writefence.Identity, Kind: writefence.Account, ID: uuid.New(), Epoch: 1})
			case "extra-scope":
				request.Command.Authorities = append(request.Command.Authorities, writefence.Authority{Issuer: writefence.Creation, Kind: writefence.Scope, ID: uuid.New(), Epoch: 1})
			case "missing-scope":
				request.Command.Authorities = append(request.Command.Authorities[:2], request.Command.Authorities[3:]...)
			case "foreign-project":
				request.ProjectID = uuid.New()
			case "agent-grant":
				request.Command.Authorities = append(request.Command.Authorities, writefence.Authority{Issuer: writefence.Creation, Kind: writefence.AgentGrant, ID: uuid.New(), Epoch: 1})
			case "unknown-action":
				request.Command.Action = "anything.execute"
			case "wrong-owner":
				request.Target = writefence.WriterMedia
			}
			var calls []string
			service := workspacesource.NewAdmission(creationIdentityAuthority{identity: id, calls: &calls}, creationSourceStore{calls: &calls})
			_, err := service.Admit(t.Context(), workspacesource.LegacyCredential("original", "csrf"), request)
			if !errors.Is(err, writefence.ErrInvalid) {
				t.Fatalf("unbound request accepted: %v", err)
			}
			for _, call := range calls {
				if call != "verify" {
					t.Fatalf("invalid request was admitted: %v", calls)
				}
			}
		})
	}
}

func TestCreationSourceCredentialFormattingIsRedacted(t *testing.T) {
	credential := workspacesource.AccessCredential("secret-access", "secret-csrf")
	if strings.Contains(credential.String(), "secret") || strings.Contains(credential.GoString(), "secret") {
		t.Fatal("credential was disclosed")
	}
}
