package microservices_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"gorm.io/gorm"

	"github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/grpchandoff"
	workspacesource "github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"

	promptpg "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/adapter/postgres"

	prompt "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/application"

	promptdomain "github.com/StephenQiu30/glowloom-server/backend/creation/prompt/domain"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/identitysource"
	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/adapter/pgsource"

	identityrpc "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/grpcauthority"

	identitypg "github.com/StephenQiu30/glowloom-server/backend/identity/adapter/postgres"

	identity "github.com/StephenQiu30/glowloom-server/backend/identity/application"

	coordination "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/coordination"

	pb "github.com/StephenQiu30/glowloom-server/backend/internal/contract/gen/creationhandoff"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence"

	pgfence "github.com/StephenQiu30/glowloom-server/backend/internal/platform/writefence/postgres"
)

type sourceProofProtector struct{ identity.ProofProtector }

func (sourceProofProtector) Digest(purpose string, id uuid.UUID, value string) []byte {
	h := hmac.New(sha256.New, []byte("source-pipeline-synthetic-test-key"))
	_, _ = h.Write([]byte(purpose))
	_, _ = h.Write(id[:])
	_, _ = h.Write([]byte(value))
	return h.Sum(nil)
}
func serveSourcePipeline(t *testing.T, server *grpc.Server) string {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		if err := <-done; err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Error(err)
		}
	})
	return listener.Addr().String()
}
func sourceWireCommand(command writefence.Command) *coordination.Command {
	out := &coordination.Command{Id: command.ID[:], Sha256: command.SHA256, Action: command.Action}
	issuers := map[writefence.Issuer]coordination.Issuer{writefence.Identity: coordination.Issuer_ISSUER_IDENTITY, writefence.Creation: coordination.Issuer_ISSUER_CREATION}
	kinds := map[writefence.Kind]coordination.AuthorityKind{writefence.Account: coordination.AuthorityKind_AUTHORITY_KIND_ACCOUNT, writefence.Session: coordination.AuthorityKind_AUTHORITY_KIND_SESSION, writefence.Scope: coordination.AuthorityKind_AUTHORITY_KIND_SCOPE, writefence.Project: coordination.AuthorityKind_AUTHORITY_KIND_PROJECT}
	for _, a := range command.Authorities {
		out.Authorities = append(out.Authorities, &coordination.Authority{Issuer: issuers[a.Issuer], Kind: kinds[a.Kind], Id: a.ID[:], Epoch: a.Epoch})
	}
	return out
}
func sourceFreezeWire(request prompt.FreezeInputRequest) *pb.FreezeFrozenInputRequest {
	out := &pb.FreezeFrozenInputRequest{HandoffId: request.HandoffID[:], RequestId: request.RequestID[:], Command: sourceWireCommand(request.Command), SourceCommand: sourceWireCommand(request.SourceCommand), ScopeId: request.ScopeID[:], ProjectId: request.ProjectID[:], Source: &pb.FrozenInputSource{Kind: request.Source.Kind, DraftId: request.Source.DraftID[:], Revision: request.Source.Revision}, SourceGeneration: request.SourceGeneration, ExpectedScopeRevision: request.ExpectedScopeRevision, ExpectedProjectRevision: request.ExpectedProjectRevision}
	if selection := request.TemplateSelection; selection != nil {
		out.TemplateSelection = &pb.FrozenTemplateSelection{Operation: selection.Operation, ExpectedTemplateId: selection.ExpectedTemplateID[:], ExpectedCustomizationRevision: selection.ExpectedCustomizationRevision}
		if options := selection.Outline; options != nil {
			out.TemplateSelection.Outline = &pb.FrozenOutlineOptions{ChapterCount: options.ChapterCount, Structure: options.Structure, WordCount: options.WordCount, Perspective: options.Perspective, Tone: options.Tone, CharacterScale: options.CharacterScale, ChapterLength: options.ChapterLength}
		}
	}
	return out
}

// This exercises the real private pipeline and final local transaction. Bootstrap
// remains a separate public registration/project-creation acceptance gate.
func TestCreationFreezePipelineUsesRealIdentityProofAndRestrictedSourceTransaction(t *testing.T) {
	f := newSourcePolicyFixture(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	actor, session := f.request.ActorID, f.request.SessionID
	legacyRaw := sha256.Sum256(session[:])
	legacy := base64.RawURLEncoding.EncodeToString(legacyRaw[:])
	raw, _ := base64.RawURLEncoding.DecodeString(legacy)
	tokenHash := sha256.Sum256(raw)
	csrf := base64.RawURLEncoding.EncodeToString(sourceProofProtector{}.Digest("session_csrf", session, legacy))
	csrfHash := sha256.Sum256([]byte(csrf))
	if err := f.admin.Exec(`UPDATE identity."user" SET email_key=?,email_verified_at=? WHERE id=?`, actor.String()+"@pipeline.invalid", now.Add(-time.Hour), actor).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.admin.Exec(`INSERT INTO identity.user_session(id,account_id,token_hash,credential_revision,created_at,last_active_at,absolute_expires_at,persistent,protocol_version,csrf_hash) VALUES (?,?,?,1,?,?,?,false,1,?)`, session, actor, tokenHash[:], now.Add(-time.Minute), now, now.Add(time.Hour), csrfHash[:]).Error; err != nil {
		t.Fatal(err)
	}
	for _, a := range f.request.Command.Authorities {
		var control writefence.Control
		if a.Issuer == writefence.Identity {
			a.Epoch = 0
			control = completedAuthority(t, pgfence.NewAuthorityStore(f.identity), a, false).Control
		} else {
			var original struct{ CoordinationID uuid.UUID }
			if err := f.admin.Raw(`SELECT coordination_id FROM infra.authority_head WHERE service_owner='creation' AND issuer='creation' AND authority_kind=? AND authority_id=?`, a.Kind, a.ID).Scan(&original).Error; err != nil {
				t.Fatal(err)
			}
			state, err := pgfence.NewAuthorityStore(f.creation).LoadCoordination(t.Context(), original.CoordinationID)
			if err != nil {
				t.Fatal(err)
			}
			control = state.Control
		}
		for _, db := range []*gorm.DB{f.identity, f.creation, f.media, f.execution} {
			if err := pgfence.New(db).Install(t.Context(), control); err != nil {
				t.Fatal(err)
			}
		}
	}
	tokens := map[writefence.Writer]string{}
	for _, writer := range writefence.Writers() {
		tokens[writer] = strings.Repeat(string(writer)+"-", 10)
	}
	auth := identity.NewEmailAuth(identitypg.NewStore(f.identity), sourceProofProtector{}, identity.EmailPolicy{}, func() time.Time { return now })
	identityServer, err := identityrpc.NewServer(auth, identitypg.NewSessionAdmissionStore(f.identity), tokens, func() time.Time { return now }, insecure.NewCredentials())
	if err != nil {
		t.Fatal(err)
	}
	identityAddress := serveSourcePipeline(t, identityServer)
	executionIdentity, err := identityrpc.NewClient(writefence.WriterExecution, identityrpc.Target{Address: identityAddress, Token: tokens[writefence.WriterExecution], Transport: insecure.NewCredentials()})
	if err != nil {
		t.Fatal(err)
	}
	creatorIdentity, err := identitysource.New(identityAddress, tokens[writefence.WriterCreation], insecure.NewCredentials())
	if err != nil {
		t.Fatal(err)
	}
	draft := promptdomain.Draft{SchemaVersion: 1, Document: promptdomain.Document{Type: "doc", Content: []promptdomain.Paragraph{{Type: "paragraph", Content: []promptdomain.Inline{{Type: "text", Text: "source pipeline"}}}}}, References: []promptdomain.Reference{}, Params: map[string]any{}, LabelState: promptdomain.LabelState{HighWater: map[string]int64{"image": 0, "video": 0, "audio": 0, "subject": 0}, Allocations: map[string]promptdomain.LabelAllocation{}}}
	modelID, mode := uuid.New(), "text"
	modelVersion := modelID.String()
	draft.ModelVersion, draft.Mode = &modelVersion, &mode
	draft.Params["output_count"] = float64(3)
	draftID := uuid.New()
	draftJSON, _ := json.Marshal(draft)
	dh, ih := draft.Hashes()
	if err := f.admin.Exec(`INSERT INTO workspace.prompt_draft(id,project_id,actor_id,revision,draft,draft_hash,input_hash) VALUES(?,?,?,1,?::jsonb,?,?)`, draftID, f.request.ProjectID, actor, string(draftJSON), dh, ih).Error; err != nil {
		t.Fatal(err)
	}
	request := prompt.FreezeInputRequest{HandoffID: uuid.New(), RequestID: uuid.New(), Command: f.request.Command, ActorID: actor, SessionID: session, ScopeID: f.request.ScopeID, ProjectID: f.request.ProjectID, Source: promptdomain.InputSource{Kind: "prompt_draft", DraftID: draftID, Revision: 1}, SourceGeneration: 1, ExpectedScopeRevision: 1, ExpectedProjectRevision: 1, Pins: []promptdomain.FrozenPin{}}
	definition, _ := promptdomain.DefinitionFor("skill_draft")
	preferenceRevision := int64(1)
	request.TemplateSelection = &prompt.TemplateRequest{Operation: definition.Operation, ExpectedTemplateID: definition.TemplateID, ExpectedCustomizationRevision: &preferenceRevision}
	var personal struct{ PhysicalScopeID uuid.UUID }
	if err := f.creation.Raw(`SELECT physical_scope_id FROM workspace.creation_scope WHERE id=?`, request.ScopeID).Scan(&personal).Error; err != nil {
		t.Fatal(err)
	}
	preferenceID := uuid.New()
	if err := f.creation.Exec(`INSERT INTO workspace.prompt_customization(id,org_id,owner_id,operation,mode,content,base_template_id,revision) VALUES(?,?,?,?,'append','原冻结个人偏好',?,1)`, preferenceID, personal.PhysicalScopeID, actor, definition.Operation, definition.TemplateID).Error; err != nil {
		t.Fatal(err)
	}
	request.SourceCommand = writefence.Command{ID: uuid.NewSHA1(request.HandoffID, []byte("creation.frozen_input.freeze")), Action: "creation.frozen_input.freeze", Authorities: append([]writefence.Authority(nil), request.Command.Authorities...)}
	request.SourceCommand.SHA256, err = request.SourceCommandSHA256()
	if err != nil {
		t.Fatal(err)
	}
	inputs := promptpg.NewFrozenInputStore(f.creation, nil)
	freezer := workspacesource.NewFrozenInputAdmission(workspacesource.NewProofAdmission(creatorIdentity, pgsource.New(f.creation)), pgfence.New(f.creation), inputs)
	compiler := workspacesource.NewFrozenInputCompilationAdmission(workspacesource.NewProofAdmission(creatorIdentity, pgsource.New(f.creation)), pgfence.New(f.creation), inputs, inputs)
	sourceServer, err := grpchandoff.NewCompilingServer(inputs, freezer, compiler, tokens[writefence.WriterExecution], insecure.NewCredentials())
	if err != nil {
		t.Fatal(err)
	}
	connection, err := grpc.NewClient(serveSourcePipeline(t, sourceServer), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDisableRetry())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = connection.Close() }()
	call := func(wire *pb.FreezeFrozenInputRequest) (*pb.FrozenInput, error) {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+tokens[writefence.WriterExecution]))
		return pb.NewFrozenInputsClient(connection).FreezeFrozenInput(ctx, wire)
	}
	if _, err := call(sourceFreezeWire(request)); err == nil {
		t.Fatal("service token substituted for missing original Identity admission")
	}
	if _, err := pgfence.New(f.creation).Load(t.Context(), request.SourceCommand.ID); !errors.Is(err, writefence.ErrNotFound) {
		t.Fatalf("missing consumer proof created a local source command: %v", err)
	}
	if _, err := executionIdentity.AdmitSessionCommand(t.Context(), identityrpc.LegacyCredential(legacy, csrf), request.Command, writefence.WriterExecution); err != nil {
		t.Fatal("real effect-owner Identity admission", err)
	}
	first, err := call(sourceFreezeWire(request))
	if err != nil {
		t.Fatal("freeze through private policy pipeline", err)
	}
	var organization struct{ OrgID uuid.UUID }
	if err := f.admin.Raw(`SELECT org_id FROM workspace.project WHERE id=?`, request.ProjectID).Scan(&organization).Error; err != nil {
		t.Fatal(err)
	}
	if string(first.OrgId) != string(organization.OrgID[:]) || organization.OrgID == request.ScopeID {
		t.Fatal("physical project organization was replaced with scope/caller identity")
	}
	unknownIdentity := sourceFreezeWire(request)
	unknownIdentity.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
	if _, err := call(unknownIdentity); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("unknown caller identity extension was accepted: %v", err)
	}
	foreignScope := sourceFreezeWire(request)
	foreignID := uuid.New()
	foreignScope.ScopeId = foreignID[:]
	if _, err := call(foreignScope); status.Code(err) != codes.InvalidArgument {
		t.Fatalf("scope selection substituted for the original command: %v", err)
	}
	second, err := call(sourceFreezeWire(request))
	if err != nil || !proto.Equal(first, second) {
		t.Fatalf("original receipt replay changed: %v", err)
	}
	if _, err := creatorIdentity.ResolveSourceAdmission(t.Context(), request.SourceCommand, writefence.WriterExecution); err == nil {
		t.Fatal("source responsibility impersonated the original consumer admission")
	}
	if _, err := creatorIdentity.ResolveSourceAdmission(t.Context(), request.Command, writefence.WriterMedia); err == nil {
		t.Fatal("same command changed original target")
	}
	changed := request
	changed.SourceGeneration = 2
	changed.SourceCommand.SHA256, _ = changed.SourceCommandSHA256()
	if _, err := call(sourceFreezeWire(changed)); status.Code(err) != codes.AlreadyExists {
		t.Fatalf("same consumer/source identities accepted another selection: %v", err)
	}
	record, err := pgfence.New(f.creation).Load(t.Context(), request.SourceCommand.ID)
	if err != nil || record.State != writefence.Committed {
		t.Fatalf("source did not commit its own exact receipt: %+v %v", record, err)
	}
	if _, err := pgfence.New(f.creation).Load(t.Context(), request.Command.ID); !errors.Is(err, writefence.ErrNotFound) {
		t.Fatalf("consumer command became a Creation effect: %v", err)
	}
	var tasks, events int
	if err := f.admin.Raw(`SELECT count(*) FROM infra.task_work WHERE service_owner='creation' AND business_id=?`, request.HandoffID.String()).Scan(&tasks).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.admin.Raw(`SELECT count(*) FROM infra.outbox WHERE service_owner='creation' AND id=?`, uuid.NewSHA1(request.HandoffID, []byte("creation.frozen_input.frozen"))).Scan(&events).Error; err != nil {
		t.Fatal(err)
	}
	if tasks != 1 || events != 1 {
		t.Fatalf("business/task/outbox replay split: %d/%d", tasks, events)
	}
	var snapshot promptdomain.FrozenInput
	if json.Unmarshal(first.CanonicalSnapshotJson, &snapshot) != nil || snapshot.CompilationInputs == nil || snapshot.CompilationInputs.Template == nil || snapshot.CompilationInputs.Template.Customization == nil || snapshot.CompilationInputs.Template.Customization.ID != preferenceID || snapshot.CompilationInputs.ProjectName != "source policy fixture" {
		t.Fatal("Freeze omitted actual template/preference/project context")
	}
	if err := f.creation.Exec(`UPDATE workspace.prompt_customization SET content='后来个人偏好',revision=2 WHERE id=?`, preferenceID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.creation.Exec(`UPDATE workspace.project SET name='后来项目名',revision=revision+1 WHERE id=?`, request.ProjectID).Error; err != nil {
		t.Fatal(err)
	}
	compileRequest := &pb.CompileFrozenInputRequest{Key: first.Key, CatalogContext: &pb.FrozenCatalogContext{ModelVersionId: modelID[:], ModelKey: "immutable-test-model", Capability: "text.structured", Mode: mode}}
	compileCall := func(wire *pb.CompileFrozenInputRequest) (*pb.FrozenInputCompilation, error) {
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("authorization", "Bearer "+tokens[writefence.WriterExecution]))
		return pb.NewFrozenInputsClient(connection).CompileFrozenInput(ctx, wire)
	}
	compiled, err := compileCall(compileRequest)
	if err != nil || compiled == nil || !strings.Contains(compiled.FinalPrompt, "原冻结个人偏好") || strings.Contains(compiled.FinalPrompt, "后来个人偏好") || compiled.OutputCount != 3 || compiled.TemplateEvidence == nil || compiled.TemplateEvidence.CustomizationRevision != 1 || compiled.OriginalPrompt != "source pipeline" {
		t.Fatalf("private compile failed to use original snapshot: %+v %v", compiled, err)
	}
	if strings.Contains(string(compiled.ParamsJson), "output_count") {
		t.Fatal("output_count remained in provider params")
	}
	changedContext := proto.Clone(compileRequest).(*pb.CompileFrozenInputRequest)
	changedContext.CatalogContext.ModelKey = "another-immutable-context"
	if _, err := compileCall(changedContext); status.Code(err) != codes.AlreadyExists {
		t.Fatal("same compile ID accepted different context", err)
	}
	unknownCompile := proto.Clone(compileRequest).(*pb.CompileFrozenInputRequest)
	unknownCompile.CatalogContext.ProtoReflect().SetUnknown([]byte{0xf8, 0x07, 0x01})
	if _, err := compileCall(unknownCompile); status.Code(err) != codes.InvalidArgument {
		t.Fatal("compiler accepted unknown context override", err)
	}
	compileRecord, err := pgfence.New(f.creation).Load(t.Context(), uuid.NewSHA1(request.HandoffID, []byte("creation.frozen_input.compile")))
	if err != nil || compileRecord.State != writefence.Committed {
		t.Fatal("compiler omitted durable final receipt", compileRecord, err)
	}
	var compileEvents int
	if err := f.creation.Raw(`SELECT count(*) FROM infra.outbox WHERE id=?`, uuid.NewSHA1(request.HandoffID, []byte("creation.frozen_input.compiled"))).Scan(&compileEvents).Error; err != nil || compileEvents != 1 {
		t.Fatal("compile receipt/outbox transaction split", compileEvents, err)
	}
	if err := f.admin.Exec(`UPDATE identity.user_session SET revoked_at=? WHERE id=?`, now, session).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := call(sourceFreezeWire(request)); err == nil {
		t.Fatal("revoked session gained a fresh source-policy permit")
	}
	identityServer.Stop()
	// A lost compile ACK is recovered from the exact original local receipt even
	// after Identity is unavailable; this read grants no new source adoption.
	recovered, err := compileCall(compileRequest)
	if err != nil || !proto.Equal(compiled, recovered) {
		t.Fatal("lost ACK/reconstructed request changed committed compilation or requested new policy", recovered, err)
	}
}
