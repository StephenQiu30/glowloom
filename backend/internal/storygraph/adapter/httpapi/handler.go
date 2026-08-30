package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/StephenQiu30/lanverse/backend/internal/access/authentication"
	platformhttp "github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	storygraphapp "github.com/StephenQiu30/lanverse/backend/internal/storygraph/application"
)

type Service interface {
	Version(context.Context, storygraphapp.Actor, storygraphapp.VersionQuery) (storygraphapp.VersionResult, error)
	Lens(context.Context, storygraphapp.Actor, storygraphapp.LensQuery) (storygraphapp.SubgraphResult, error)
	Trace(context.Context, storygraphapp.Actor, storygraphapp.TraceQuery) (storygraphapp.SubgraphResult, error)
	Diff(context.Context, storygraphapp.Actor, storygraphapp.DiffQuery) (storygraphapp.DiffResult, error)
}

type Authenticator interface {
	Authenticate(*http.Request) (authentication.Claims, error)
}

type Handler struct {
	service       Service
	authenticator Authenticator
}

func New(service Service, authenticator Authenticator) *Handler {
	return &Handler{service: service, authenticator: authenticator}
}

func (handler *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects/{project_id}/storygraph/current", handler.current)
	mux.HandleFunc("GET /api/projects/{project_id}/storygraph/versions/{version_id}", handler.exact)
	mux.HandleFunc("GET /api/projects/{project_id}/storygraph/versions/{version_ref}/lens", handler.lens)
	mux.HandleFunc("GET /api/projects/{project_id}/storygraph/versions/{version_ref}/nodes/{story_node_key}/trace", handler.trace)
	mux.HandleFunc("GET /api/projects/{project_id}/storygraph/diff", handler.diff)
}

func (handler *Handler) current(writer http.ResponseWriter, request *http.Request) {
	handler.version(writer, request, storygraphapp.VersionRefCurrent)
}

func (handler *Handler) exact(writer http.ResponseWriter, request *http.Request) {
	handler.version(writer, request, request.PathValue("version_id"))
}

func (handler *Handler) version(writer http.ResponseWriter, request *http.Request, versionRef string) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	value, err := handler.service.Version(request.Context(), actor, storygraphapp.VersionQuery{
		ProjectID: request.PathValue("project_id"), VersionRef: versionRef,
	})
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": presentVersion(value)})
}

func (handler *Handler) lens(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	depth, depthOK := requiredInteger(request.URL.Query().Get("depth"))
	limit, limitOK := requiredInteger(request.URL.Query().Get("limit"))
	if !depthOK || !limitOK {
		writeValidation(writer, request)
		return
	}
	query := storygraphapp.LensQuery{
		ProjectID: request.PathValue("project_id"), VersionRef: request.PathValue("version_ref"),
		Lens: request.URL.Query().Get("lens"), ScopeKind: request.URL.Query().Get("scope_kind"),
		ScopeID: request.URL.Query().Get("scope_id"), Depth: depth, Limit: limit,
		Cursor: request.URL.Query().Get("cursor"),
	}
	result, err := handler.service.Lens(request.Context(), actor, query)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": presentSubgraph(result)})
}

func (handler *Handler) trace(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	depth, depthOK := requiredInteger(request.URL.Query().Get("depth"))
	limit, limitOK := requiredInteger(request.URL.Query().Get("limit"))
	if !depthOK || !limitOK {
		writeValidation(writer, request)
		return
	}
	query := storygraphapp.TraceQuery{
		ProjectID: request.PathValue("project_id"), VersionRef: request.PathValue("version_ref"),
		StoryNodeKey: request.PathValue("story_node_key"), Direction: request.URL.Query().Get("direction"),
		Depth: depth, Limit: limit, Cursor: request.URL.Query().Get("cursor"),
	}
	result, err := handler.service.Trace(request.Context(), actor, query)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": presentSubgraph(result)})
}

func (handler *Handler) diff(writer http.ResponseWriter, request *http.Request) {
	actor, ok := handler.actor(writer, request)
	if !ok {
		return
	}
	limit, limitOK := requiredInteger(request.URL.Query().Get("limit"))
	if !limitOK {
		writeValidation(writer, request)
		return
	}
	query := storygraphapp.DiffQuery{
		ProjectID:     request.PathValue("project_id"),
		BaseVersionID: request.URL.Query().Get("base_version_id"), TargetVersionID: request.URL.Query().Get("target_version_id"),
		Limit: limit, Cursor: request.URL.Query().Get("cursor"),
	}
	result, err := handler.service.Diff(request.Context(), actor, query)
	if err != nil {
		handler.writeError(writer, request, err)
		return
	}
	platformhttp.WriteJSON(writer, http.StatusOK, map[string]any{"data": presentDiff(result)})
}

func (handler *Handler) actor(writer http.ResponseWriter, request *http.Request) (storygraphapp.Actor, bool) {
	claims, err := handler.authenticator.Authenticate(request)
	if err != nil {
		handler.writeError(writer, request, &storygraphapp.Error{
			Code: "unauthenticated", Message: "Invalid credentials", Status: http.StatusUnauthorized, NextAction: "login",
		})
		return storygraphapp.Actor{}, false
	}
	return storygraphapp.Actor{UserID: claims.UserID, TokenVersion: claims.TokenVersion}, true
}

func (handler *Handler) writeError(writer http.ResponseWriter, request *http.Request, err error) {
	var value *storygraphapp.Error
	if !errors.As(err, &value) {
		value = &storygraphapp.Error{Code: "internal_error", Message: "Internal server error", Status: http.StatusInternalServerError}
	}
	platformhttp.WriteProblem(writer, request, platformhttp.Problem{
		Code: value.Code, Message: value.Message, Status: value.Status,
		NextAction: value.NextAction, Details: value.Details,
	})
}

func presentVersion(result storygraphapp.VersionResult) map[string]any {
	value := result.Version
	return map[string]any{
		"id": value.ID, "workspace_id": value.WorkspaceID, "project_id": value.ProjectID,
		"version_no": value.VersionNo, "parent_version_id": value.ParentVersionID,
		"parent_content_hash": value.ParentContentHash, "source_revision_id": value.SourceRevisionID,
		"source_revision_hash": value.SourceRevisionHash, "owner_set_hash": value.OwnerSetHash,
		"schema_version": value.SchemaVersion, "topology_hash": value.TopologyHash,
		"content_hash": value.ContentHash, "status": value.Status,
		"published_at": value.PublishedAt, "created_by": value.CreatedBy, "created_at": value.CreatedAt,
		"node_count": len(value.Nodes), "edge_count": len(value.Edges),
		"compiled_from": result.CompiledFrom, "stale": result.Stale,
	}
}

func presentSubgraph(value storygraphapp.SubgraphResult) map[string]any {
	return map[string]any{
		"version_id": value.VersionID, "version_no": value.VersionNo, "content_hash": value.ContentHash,
		"lens": nullable(value.Lens), "scope": map[string]any{"kind": value.ScopeKind, "id": value.ScopeID},
		"direction": nullable(value.Direction), "depth": value.Depth,
		"nodes": value.Nodes, "edges": value.Edges, "truncated": value.Truncated,
		"next_cursor": nullable(value.NextCursor), "result_hash": value.ResultHash,
	}
}

func presentDiff(value storygraphapp.DiffResult) map[string]any {
	nodes := make([]map[string]any, len(value.NodeChanges))
	for index, change := range value.NodeChanges {
		nodes[index] = map[string]any{
			"story_node_key": change.StoryNodeKey, "change_type": change.ChangeType,
			"before_content_hash": nullable(change.BeforeContentHash), "after_content_hash": nullable(change.AfterContentHash),
		}
	}
	edges := make([]map[string]any, len(value.EdgeChanges))
	for index, change := range value.EdgeChanges {
		edges[index] = map[string]any{
			"edge_key": change.EdgeKey, "change_type": change.ChangeType,
			"before_content_hash": nullable(change.BeforeContentHash), "after_content_hash": nullable(change.AfterContentHash),
		}
	}
	return map[string]any{
		"base_version_id": value.BaseVersionID, "base_content_hash": value.BaseContentHash,
		"target_version_id": value.TargetVersionID, "target_content_hash": value.TargetContentHash,
		"node_changes": nodes, "edge_changes": edges, "truncated": value.Truncated,
		"next_cursor": nullable(value.NextCursor), "result_hash": value.ResultHash,
	}
}

func requiredInteger(raw string) (int, bool) {
	if strings.TrimSpace(raw) == "" {
		return 0, false
	}
	value, err := strconv.Atoi(raw)
	return value, err == nil
}

func writeValidation(writer http.ResponseWriter, request *http.Request) {
	platformhttp.WriteProblem(writer, request, platformhttp.Problem{
		Code: "validation_failed", Message: "Request validation failed", Status: http.StatusUnprocessableEntity,
	})
}

func nullable(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}
