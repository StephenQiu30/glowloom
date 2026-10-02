package http

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	identityhttp "github.com/StephenQiu30/lanverse/backend/internal/identity/adapter/http"
	"github.com/StephenQiu30/lanverse/backend/internal/platform/httpapi"
)

func (h *Handler) control(c *gin.Context, action string, look bool) {
	var body ControlRequest
	if !readJSON(c, &body) {
		return
	}
	input, ok := command(c, action, body.ExpectedRevision)
	if !ok {
		return
	}
	if look && !lookID(c, &input) {
		return
	}
	input.AcknowledgedImpact = body.AcknowledgedImpact
	h.change(c, input)
}
func (h *Handler) look(c *gin.Context, action string, existing bool) {
	var body LookRequest
	if !readJSON(c, &body) {
		return
	}
	input, ok := command(c, action, body.ExpectedRevision)
	if !ok {
		return
	}
	if existing && !lookID(c, &input) {
		return
	}
	input.Look = &body.Look
	h.change(c, input)
}

// Create Create one manual candidate identity.
// @ID createBibleEntry
// @Tags bible
// @Accept json
// @Param pid path string true "Project UUID"
// @Param kind path string true "Identity kind" Enums(character,location,prop)
// @Param Idempotency-Key header string true "Permanent UUID command key"
// @Param body body ContentRequest true "Closed command fields"
// @Success 200 {object} application.Receipt
// @Failure 403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind} [post]
func (h *Handler) Create(c *gin.Context) {
	var body ContentRequest
	if !readJSON(c, &body) {
		return
	}
	input, ok := command(c, "create", body.ExpectedRevision)
	if !ok {
		return
	}
	input.Character = body.Character
	input.Location = body.Location
	input.Prop = body.Prop
	h.change(c, input)
}

// Update Publish a complete immutable definition version.
// @ID updateBibleEntry
// @Tags bible
// @Accept json
// @Param pid path string true "Project UUID"
// @Param kind path string true "Identity kind" Enums(character,location,prop)
// @Param id path string true "Identity UUID"
// @Param Idempotency-Key header string true "Permanent UUID command key"
// @Param body body ContentRequest true "Closed command fields"
// @Success 200 {object} application.Receipt
// @Failure 403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind}/{id} [put]
func (h *Handler) Update(c *gin.Context) {
	var body ContentRequest
	if !readJSON(c, &body) {
		return
	}
	input, ok := command(c, "update", body.ExpectedRevision)
	if !ok {
		return
	}
	input.Character = body.Character
	input.Location = body.Location
	input.Prop = body.Prop
	h.change(c, input)
}

// Confirm Explicitly confirm the current version.
// @ID confirmBibleEntry
// @Tags bible
// @Accept json
// @Param pid path string true "Project UUID"
// @Param kind path string true "Identity kind" Enums(character,location,prop)
// @Param id path string true "Identity UUID"
// @Param Idempotency-Key header string true "Permanent UUID command key"
// @Param body body ControlRequest true "Closed command fields"
// @Success 200 {object} application.Receipt
// @Failure 403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind}/{id}/confirm [post]
func (h *Handler) Confirm(c *gin.Context) { h.control(c, "confirm", false) }

// Delete Soft-delete an identity with actual impact evidence.
// @ID deleteBibleEntry
// @Tags bible
// @Accept json
// @Param pid path string true "Project UUID"
// @Param kind path string true "Identity kind" Enums(character,location,prop)
// @Param id path string true "Identity UUID"
// @Param Idempotency-Key header string true "Permanent UUID command key"
// @Param body body ControlRequest true "Closed command fields"
// @Success 200 {object} application.Receipt
// @Failure 403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind}/{id} [delete]
func (h *Handler) Delete(c *gin.Context) { h.control(c, "delete", false) }

// Restore Restore a soft-deleted stable identity.
// @ID restoreBibleEntry
// @Tags bible
// @Accept json
// @Param pid path string true "Project UUID"
// @Param kind path string true "Identity kind" Enums(character,location,prop)
// @Param id path string true "Identity UUID"
// @Param Idempotency-Key header string true "Permanent UUID command key"
// @Param body body ControlRequest true "Closed command fields"
// @Success 200 {object} application.Receipt
// @Failure 403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind}/{id}/restore [post]
func (h *Handler) Restore(c *gin.Context) { h.control(c, "restore", false) }

// CreateLook Publish a new appearance in a complete character version.
// @ID createBibleLook
// @Tags bible
// @Accept json
// @Param pid path string true "Project UUID"
// @Param kind path string true "Identity kind" Enums(character,location,prop)
// @Param id path string true "Identity UUID"
// @Param Idempotency-Key header string true "Permanent UUID command key"
// @Param body body LookRequest true "Closed command fields"
// @Success 200 {object} application.Receipt
// @Failure 403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind}/{id}/looks [post]
func (h *Handler) CreateLook(c *gin.Context) { h.look(c, "look_create", false) }

// UpdateLook Publish changed appearance fields while preserving history.
// @ID updateBibleLook
// @Tags bible
// @Accept json
// @Param pid path string true "Project UUID"
// @Param kind path string true "Identity kind" Enums(character,location,prop)
// @Param id path string true "Identity UUID"
// @Param look path string true "Appearance UUID"
// @Param Idempotency-Key header string true "Permanent UUID command key"
// @Param body body LookRequest true "Closed command fields"
// @Success 200 {object} application.Receipt
// @Failure 403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind}/{id}/looks/{look} [put]
func (h *Handler) UpdateLook(c *gin.Context) { h.look(c, "look_update", true) }

// DeleteLook Remove an appearance from a new version with impact evidence.
// @ID deleteBibleLook
// @Tags bible
// @Accept json
// @Param pid path string true "Project UUID"
// @Param kind path string true "Identity kind" Enums(character,location,prop)
// @Param id path string true "Identity UUID"
// @Param look path string true "Appearance UUID"
// @Param Idempotency-Key header string true "Permanent UUID command key"
// @Param body body ControlRequest true "Closed command fields"
// @Success 200 {object} application.Receipt
// @Failure 403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind}/{id}/looks/{look} [delete]
func (h *Handler) DeleteLook(c *gin.Context) { h.control(c, "look_delete", true) }

// DefaultLook Select exactly one default appearance in a new version.
// @ID setBibleDefaultLook
// @Tags bible
// @Accept json
// @Param pid path string true "Project UUID"
// @Param kind path string true "Identity kind" Enums(character,location,prop)
// @Param id path string true "Identity UUID"
// @Param look path string true "Appearance UUID"
// @Param Idempotency-Key header string true "Permanent UUID command key"
// @Param body body ControlRequest true "Closed command fields"
// @Success 200 {object} application.Receipt
// @Failure 403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind}/{id}/looks/{look}/default [post]
func (h *Handler) DefaultLook(c *gin.Context) { h.control(c, "look_default", true) }

// References Freeze a complete validated image purpose set.
// @ID replaceBibleReferences
// @Tags bible
// @Accept json
// @Param pid path string true "Project UUID"
// @Param kind path string true "Identity kind" Enums(character,location,prop)
// @Param id path string true "Identity UUID"
// @Param look path string true "Appearance UUID"
// @Param Idempotency-Key header string true "Permanent UUID command key"
// @Param body body ReferencesRequest true "Closed command fields"
// @Success 200 {object} application.Receipt
// @Failure 403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind}/{id}/looks/{look}/references [put]
func (h *Handler) References(c *gin.Context) {
	var body ReferencesRequest
	if !readJSON(c, &body) {
		return
	}
	input, ok := command(c, "references", body.ExpectedRevision)
	if !ok || !lookID(c, &input) {
		return
	}
	input.References = body.References
	h.change(c, input)
}

// BindVoice Freeze a configured catalog voice or actual uploaded audio sample.
// @ID bindBibleVoice
// @Tags bible
// @Accept json
// @Param pid path string true "Project UUID"
// @Param kind path string true "Identity kind" Enums(character,location,prop)
// @Param id path string true "Identity UUID"
// @Param Idempotency-Key header string true "Permanent UUID command key"
// @Param body body VoiceRequest true "Closed command fields"
// @Success 200 {object} application.Receipt
// @Failure 403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind}/{id}/voice [put]
func (h *Handler) BindVoice(c *gin.Context) {
	var body VoiceRequest
	if !readJSON(c, &body) {
		return
	}
	input, ok := command(c, "voice_bind", body.ExpectedRevision)
	if !ok {
		return
	}
	input.Voice = &body.Voice
	h.change(c, input)
}

// UnbindVoice Publish a new version without its former voice binding.
// @ID unbindBibleVoice
// @Tags bible
// @Accept json
// @Param pid path string true "Project UUID"
// @Param kind path string true "Identity kind" Enums(character,location,prop)
// @Param id path string true "Identity UUID"
// @Param Idempotency-Key header string true "Permanent UUID command key"
// @Param body body ControlRequest true "Closed command fields"
// @Success 200 {object} application.Receipt
// @Failure 403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind}/{id}/voice [delete]
func (h *Handler) UnbindVoice(c *gin.Context) { h.control(c, "voice_unbind", false) }

// AdoptResult Adopt an actual successful extraction result through its owning port.
// @ID adoptBibleResult
// @Tags bible
// @Accept json
// @Param pid path string true "Project UUID"
// @Param kind path string true "Identity kind" Enums(character,location,prop)
// @Param id path string true "Identity UUID"
// @Param Idempotency-Key header string true "Permanent UUID command key"
// @Param body body ResultRequest true "Closed command fields"
// @Success 200 {object} application.Receipt
// @Failure 403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind}/{id}/adopt-result [post]
func (h *Handler) AdoptResult(c *gin.Context) {
	var body ResultRequest
	if !readJSON(c, &body) {
		return
	}
	input, ok := command(c, "adopt_result", body.ExpectedRevision)
	if !ok {
		return
	}
	input.OperationID = &body.OperationID
	input.OutputID = &body.OutputID
	h.change(c, input)
}

// CreateResult adopts one real extraction output into a new candidate identity.
// @ID createBibleEntryFromResult
// @Tags bible
// @Accept json
// @Param pid path string true "Project UUID"
// @Param kind path string true "Identity kind" Enums(character,location,prop)
// @Param Idempotency-Key header string true "Permanent UUID command key"
// @Param body body ResultRequest true "Authorized successful output"
// @Success 200 {object} application.Receipt
// @Failure 403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind}/adopt-result [post]
func (h *Handler) CreateResult(c *gin.Context) {
	var body ResultRequest
	if !readJSON(c, &body) {
		return
	}
	input, ok := command(c, "create_result", body.ExpectedRevision)
	if !ok {
		return
	}
	input.OperationID = &body.OperationID
	input.OutputID = &body.OutputID
	h.change(c, input)
}

// Merge preserves a reviewed redirect and both immutable historical identities.
// @ID mergeBibleCharacter
// @Tags bible
// @Accept json
// @Param pid path string true "Project UUID"
// @Param kind path string true "Character kind" Enums(character)
// @Param id path string true "Source character UUID"
// @Param Idempotency-Key header string true "Permanent UUID command key"
// @Param body body MergeRequest true "Both CAS heads and impact acknowledgment"
// @Success 200 {object} application.Receipt
// @Failure 403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind}/{id}/merge [post]
func (h *Handler) Merge(c *gin.Context) {
	var body MergeRequest
	if !readJSON(c, &body) {
		return
	}
	input, ok := command(c, "merge", body.ExpectedRevision)
	if !ok {
		return
	}
	input.TargetID = &body.TargetID
	input.ExpectedTargetRevision = body.ExpectedTargetRevision
	input.AcknowledgedImpact = body.AcknowledgedImpact
	h.change(c, input)
}

// Split creates an independent identity while preserving parent versions and old assignments.
// @ID splitBibleCharacter
// @Tags bible
// @Accept json
// @Param pid path string true "Project UUID"
// @Param kind path string true "Character kind" Enums(character)
// @Param id path string true "Parent character UUID"
// @Param Idempotency-Key header string true "Permanent UUID command key"
// @Param body body SplitRequest true "Parent CAS and independent child definition"
// @Success 200 {object} application.Receipt
// @Failure 403,404,409,413,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible/{kind}/{id}/split [post]
func (h *Handler) Split(c *gin.Context) {
	var body SplitRequest
	if !readJSON(c, &body) {
		return
	}
	input, ok := command(c, "split", body.ExpectedRevision)
	if !ok {
		return
	}
	input.Character = &body.Character
	h.change(c, input)
}

// Voices returns only real configured voices without seed writes.
// @ID listBibleVoices
// @Tags bible
// @Param pid path string true "Project UUID"
// @Param limit query int false "Page size, maximum 200"
// @Param cursor query string false "Opaque page cursor"
// @Success 200 {object} VoiceResponse
// @Failure 403,422,503 {object} httpapi.Problem
// @Router /api/projects/{pid}/bible-voices [get]
func (h *Handler) Voices(c *gin.Context) {
	pid, err := uuid.Parse(c.Param("pid"))
	if err != nil || pid == uuid.Nil {
		httpapi.WriteProblem(c, 422, "invalid_request", nil)
		return
	}
	var cursor application.VoiceCursor
	limit, ok := queryPage(c, 200, &cursor)
	if !ok {
		return
	}
	var after *application.VoiceCursor
	if c.Query("cursor") != "" {
		after = &cursor
	}
	page, err := h.service.Voices(c.Request.Context(), identityhttp.Principal(c), pid, limit, after)
	if err != nil {
		writeError(c, err)
		return
	}
	result := VoiceResponse{Voices: page.Voices}
	if page.Next != nil {
		result.NextCursor = encodeCursor(page.Next)
	}
	c.JSON(200, result)
}
