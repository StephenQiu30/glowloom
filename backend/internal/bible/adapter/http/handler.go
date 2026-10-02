// Package http exposes currently authorized typed Bible content through public REST.
package http

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
	workspacedomain "github.com/StephenQiu30/lanverse/backend/internal/workspace/domain"
)

// Handler injects the formal Bible use case; it neither creates seeds nor executes providers.
type Handler struct{ service *application.Service }

// NewHandler injects the persistent current-authorized application service.
func NewHandler(service *application.Service) *Handler { return &Handler{service: service} }

// Register installs the complete typed content routes under the protected API group.
func (h *Handler) Register(g *gin.RouterGroup) {
	g.GET("/projects/:pid/bible/:kind", h.List)
	g.POST("/projects/:pid/bible/:kind", h.Create)
	g.POST("/projects/:pid/bible/:kind/adopt-result", h.CreateResult)
	g.GET("/projects/:pid/bible/:kind/:id", h.Get)
	g.PUT("/projects/:pid/bible/:kind/:id", h.Update)
	g.GET("/projects/:pid/bible/:kind/:id/versions", h.History)
	g.GET("/projects/:pid/bible/:kind/:id/versions/:version", h.Version)
	g.POST("/projects/:pid/bible/:kind/:id/confirm", h.Confirm)
	g.DELETE("/projects/:pid/bible/:kind/:id", h.Delete)
	g.POST("/projects/:pid/bible/:kind/:id/restore", h.Restore)
	g.POST("/projects/:pid/bible/:kind/:id/merge", h.Merge)
	g.POST("/projects/:pid/bible/:kind/:id/split", h.Split)
	g.POST("/projects/:pid/bible/:kind/:id/looks", h.CreateLook)
	g.PUT("/projects/:pid/bible/:kind/:id/looks/:look", h.UpdateLook)
	g.DELETE("/projects/:pid/bible/:kind/:id/looks/:look", h.DeleteLook)
	g.POST("/projects/:pid/bible/:kind/:id/looks/:look/default", h.DefaultLook)
	g.PUT("/projects/:pid/bible/:kind/:id/looks/:look/references", h.References)
	g.PUT("/projects/:pid/bible/:kind/:id/voice", h.BindVoice)
	g.DELETE("/projects/:pid/bible/:kind/:id/voice", h.UnbindVoice)
	g.GET("/projects/:pid/bible-voices", h.Voices)
	g.POST("/projects/:pid/bible/:kind/:id/adopt-result", h.AdoptResult)
}

// ContentRequest submits one kind-specific definition without server-only evidence.
type ContentRequest struct {
	ExpectedRevision *int64                      `json:"expected_revision"`
	Character        *application.CharacterInput `json:"character,omitempty"`
	Location         *domain.LocationContent     `json:"location,omitempty"`
	Prop             *domain.PropContent         `json:"prop,omitempty"`
}

// ControlRequest binds an explicit action to its current identity and impact evidence.
type ControlRequest struct {
	ExpectedRevision   *int64                   `json:"expected_revision"`
	AcknowledgedImpact *application.ImpactProof `json:"acknowledged_impact,omitempty"`
}

// LookRequest changes only the user-editable appearance fields.
type LookRequest struct {
	ExpectedRevision *int64                `json:"expected_revision"`
	Look             application.LookInput `json:"look"`
}

// ReferencesRequest replaces one appearance's complete purpose set with formal asset identities.
type ReferencesRequest struct {
	ExpectedRevision *int64                       `json:"expected_revision"`
	References       []application.ReferenceInput `json:"references"`
}

// VoiceRequest selects an actual catalog voice or a formal sample.
type VoiceRequest struct {
	ExpectedRevision *int64                 `json:"expected_revision"`
	Voice            application.VoiceInput `json:"voice"`
}

// ResultRequest selects one successful output for adoption; clients cannot invent provenance.
type ResultRequest struct {
	ExpectedRevision *int64    `json:"expected_revision"`
	OperationID      uuid.UUID `json:"operation_id"`
	OutputID         uuid.UUID `json:"output_id"`
}

// MergeRequest identifies both reviewed character heads and real impact acknowledgment.
type MergeRequest struct {
	ExpectedRevision       *int64                   `json:"expected_revision"`
	TargetID               uuid.UUID                `json:"target_id"`
	ExpectedTargetRevision *int64                   `json:"expected_target_revision"`
	AcknowledgedImpact     *application.ImpactProof `json:"acknowledged_impact,omitempty"`
}

// SplitRequest creates independent new content from one current character identity.
type SplitRequest struct {
	ExpectedRevision *int64                     `json:"expected_revision"`
	Character        application.CharacterInput `json:"character"`
}

// PageResponse bounds entries and returns current principal scope for intent recovery.
type PageResponse struct {
	Entries        []application.Summary `json:"entries"`
	NextCursor     string                `json:"next_cursor,omitempty"`
	CurrentActorID uuid.UUID             `json:"current_actor_id"`
	CurrentOrgID   uuid.UUID             `json:"current_org_id"`
}

// HistoryResponse provides complete immutable snapshots through bounded pages.
type HistoryResponse struct {
	Versions   []domain.Version `json:"versions"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

// VoiceResponse contains only actually configured voice choices.
type VoiceResponse struct {
	Voices     []application.VoiceChoice `json:"voices"`
	NextCursor string                    `json:"next_cursor,omitempty"`
}

type strictRequest struct{ value any }

func (r *strictRequest) UnmarshalJSON(data []byte) error {
	return domain.DecodeClosedJSON(data, r.value)
}
func readJSON(c *gin.Context, out any) bool { return httpapi.Decode(c, &strictRequest{value: out}) }
func scope(c *gin.Context, entry bool) (uuid.UUID, domain.Kind, uuid.UUID, bool) {
	pid, err := uuid.Parse(c.Param("pid"))
	kind := domain.Kind(c.Param("kind"))
	if err != nil || pid == uuid.Nil || !kind.Valid() {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return uuid.Nil, "", uuid.Nil, false
	}
	id := uuid.Nil
	if entry {
		id, err = uuid.Parse(c.Param("id"))
		if err != nil || id == uuid.Nil {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return uuid.Nil, "", uuid.Nil, false
		}
	}
	return pid, kind, id, true
}
func command(c *gin.Context, action string, revision *int64) (application.Command, bool) {
	pid, kind, id, ok := scope(c, action != "create" && action != "create_result")
	if !ok {
		return application.Command{}, false
	}
	key, err := uuid.Parse(c.GetHeader("Idempotency-Key"))
	request, traceErr := uuid.Parse(c.GetString("request_id"))
	if err != nil || traceErr != nil || revision == nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return application.Command{}, false
	}
	return application.Command{ProjectID: pid, Key: key, RequestID: request, Kind: kind, Action: action, EntryID: id, ExpectedRevision: *revision}, true
}
func lookID(c *gin.Context, input *application.Command) bool {
	id, err := uuid.Parse(c.Param("look"))
	if err != nil || id == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return false
	}
	input.LookID = &id
	return true
}
func (h *Handler) change(c *gin.Context, input application.Command) {
	result, err := h.service.Change(c.Request.Context(), identityhttp.Principal(c), input)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, result)
}
func writeError(c *gin.Context, err error) {
	var impact *application.ImpactConflict
	switch {
	case errors.As(err, &impact):
		httpapi.WriteProblem(c, 409, "impact_acknowledgment_required", map[string]any{"impact": impact.Proof})
	case errors.Is(err, identityapp.ErrForbidden):
		httpapi.WriteProblem(c, 403, "forbidden", nil)
	case errors.Is(err, application.ErrNotFound):
		httpapi.WriteProblem(c, 404, "not_found", nil)
	case errors.Is(err, application.ErrIdempotencyConflict):
		httpapi.WriteProblem(c, 409, "idempotency_conflict", nil)
	case errors.Is(err, application.ErrConflict), errors.Is(err, workspacedomain.ErrProjectRevisionConflict), errors.Is(err, workspacedomain.ErrProjectStateConflict):
		httpapi.WriteProblem(c, 409, "revision_conflict", nil)
	case errors.Is(err, domain.ErrInvalidContent):
		httpapi.WriteProblem(c, 422, "invalid_bible_content", nil)
	default:
		httpapi.WriteProblem(c, 503, "context_unavailable", nil)
	}
}
func queryPage(c *gin.Context, maximum int, out any) (int, bool) {
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maximum {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return 0, false
		}
		limit = n
	}
	if raw := c.Query("cursor"); raw != "" {
		data, err := base64.RawURLEncoding.DecodeString(raw)
		if err != nil || len(data) > 1024 || domain.DecodeClosedJSON(data, out) != nil {
			httpapi.WriteProblem(c, 422, "invalid_request", nil)
			return 0, false
		}
	}
	return limit, true
}
func encodeCursor(cursor any) string {
	b, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// List returns a bounded current project Bible list.
// @ID listBibleEntries
// @Tags bible
// @Param pid path string true "Project UUID"
// @Param kind path string true "Identity kind" Enums(character,location,prop)
// @Param limit query int false "Page size, maximum 200"
// @Param cursor query string false "Opaque page cursor"
// @Success 200 {object} PageResponse
// @Failure 403,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind} [get]
func (h *Handler) List(c *gin.Context) {
	pid, kind, _, ok := scope(c, false)
	if !ok {
		return
	}
	var cursor application.ListCursor
	limit, ok := queryPage(c, 200, &cursor)
	if !ok {
		return
	}
	var after *application.ListCursor
	if c.Query("cursor") != "" {
		after = &cursor
	}
	page, err := h.service.List(c.Request.Context(), identityhttp.Principal(c), application.ListInput{ProjectID: pid, Kind: kind, Limit: limit, After: after})
	if err != nil {
		writeError(c, err)
		return
	}
	result := PageResponse{Entries: page.Entries, CurrentActorID: page.CurrentActorID, CurrentOrgID: page.CurrentOrgID}
	if page.Next != nil {
		result.NextCursor = encodeCursor(page.Next)
	}
	c.JSON(200, result)
}

// Get returns current content and the stable identity's redirect resolution.
// @ID getBibleEntry
// @Tags bible
// @Param pid path string true "Project UUID"
// @Param kind path string true "Identity kind" Enums(character,location,prop)
// @Param id path string true "Identity UUID"
// @Success 200 {object} application.Detail
// @Failure 403,404,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind}/{id} [get]
func (h *Handler) Get(c *gin.Context) {
	pid, kind, id, ok := scope(c, true)
	if !ok {
		return
	}
	result, err := h.service.Find(c.Request.Context(), identityhttp.Principal(c), pid, kind, id)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, result)
}

// History returns complete historical versions without changing current pointers.
// @ID listBibleVersions
// @Tags bible
// @Param pid path string true "Project UUID"
// @Param kind path string true "Identity kind" Enums(character,location,prop)
// @Param id path string true "Identity UUID"
// @Param limit query int false "Page size, maximum 50"
// @Param cursor query string false "Opaque page cursor"
// @Success 200 {object} HistoryResponse
// @Failure 403,404,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind}/{id}/versions [get]
func (h *Handler) History(c *gin.Context) {
	pid, kind, id, ok := scope(c, true)
	if !ok {
		return
	}
	var cursor application.HistoryCursor
	limit, ok := queryPage(c, 50, &cursor)
	if !ok {
		return
	}
	var after *application.HistoryCursor
	if c.Query("cursor") != "" {
		after = &cursor
	}
	page, err := h.service.History(c.Request.Context(), identityhttp.Principal(c), pid, kind, id, limit, after)
	if err != nil {
		writeError(c, err)
		return
	}
	result := HistoryResponse{Versions: page.Versions}
	if page.Next != nil {
		result.NextCursor = encodeCursor(page.Next)
	}
	c.JSON(200, result)
}

// Version returns an exact pinned immutable historical snapshot.
// @ID getBibleVersion
// @Tags bible
// @Param pid path string true "Project UUID"
// @Param kind path string true "Identity kind" Enums(character,location,prop)
// @Param id path string true "Identity UUID"
// @Param version path string true "Version UUID"
// @Success 200 {object} domain.Version
// @Failure 403,404,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind}/{id}/versions/{version} [get]
func (h *Handler) Version(c *gin.Context) {
	pid, kind, id, ok := scope(c, true)
	if !ok {
		return
	}
	version, err := uuid.Parse(c.Param("version"))
	if err != nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	result, err := h.service.Version(c.Request.Context(), identityhttp.Principal(c), pid, kind, id, version)
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(200, result)
}
