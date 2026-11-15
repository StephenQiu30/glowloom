package httpprojects

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/creation/workspace/application/source"
)

// ProjectDetailResponse contains safe settings and actual persisted recovery facts.
type ProjectDetailResponse struct {
	LastModifiedAt      *time.Time        `json:"last_modified_at,omitempty"`
	LastModifiedBasis   *string           `json:"last_modified_basis,omitempty"`
	CoverAssetID        *uuid.UUID        `json:"cover_asset_id" extensions:"x-nullable"`
	CoverUnavailable    bool              `json:"cover_unavailable"`
	ID                  uuid.UUID         `json:"id"`
	CanvasID            *uuid.UUID        `json:"canvas_id" extensions:"x-nullable"`
	StyleKey            *string           `json:"style_key" extensions:"x-nullable"`
	Name                string            `json:"name"`
	Description         string            `json:"description"`
	AspectRatio         string            `json:"aspect_ratio"`
	StyleType           string            `json:"style_type"`
	StyleSubtype        string            `json:"style_subtype,omitempty"`
	StylePresetID       *uuid.UUID        `json:"style_preset_id" extensions:"x-nullable"`
	Resolution          string            `json:"resolution"`
	AllowOverseasModels bool              `json:"allow_overseas_models"`
	DefaultModels       map[string]string `json:"default_models" swaggertype:"object,string"`
	Status              string            `json:"status"`
	IsDelete            bool              `json:"is_delete"`
	ArchivedAt          *time.Time        `json:"archived_at" extensions:"x-nullable"`
	DeleteTime          *time.Time        `json:"delete_time" extensions:"x-nullable"`
	PurgeAfter          *time.Time        `json:"purge_after" extensions:"x-nullable"`
	Revision            int64             `json:"revision"`
	CreateTime          time.Time         `json:"create_time"`
	UpdateTime          time.Time         `json:"update_time"`
}

// WriteProjectDetails is the single original legacy/native response formatter.
func WriteProjectDetails(c *gin.Context, s source.ProjectCloseDetails) {
	p := s.Project
	var preset, canvas *uuid.UUID
	if p.StylePresetID != uuid.Nil {
		id := p.StylePresetID
		preset = &id
	}
	if p.CanvasID != uuid.Nil {
		id := p.CanvasID
		canvas = &id
	}
	response := ProjectDetailResponse{LastModifiedAt: s.LastModifiedAt, LastModifiedBasis: s.LastModifiedBasis, CoverAssetID: p.CoverAssetID, CoverUnavailable: s.CoverUnavailable, ID: p.ID, CanvasID: canvas, StyleKey: p.StyleKey, Name: p.Name, Description: p.Description, AspectRatio: p.AspectRatio, StyleType: p.StyleType, StyleSubtype: p.StyleSubtype, StylePresetID: preset, Resolution: p.Resolution, AllowOverseasModels: p.AllowOverseasModels, DefaultModels: s.DefaultModels, Status: p.Status, IsDelete: p.IsDelete, ArchivedAt: p.ArchivedAt, DeleteTime: p.DeleteTime, PurgeAfter: p.PurgeAfter, Revision: p.Revision, CreateTime: p.CreateTime, UpdateTime: p.UpdateTime}
	c.Header("Cache-Control", "private, no-store")
	c.JSON(200, response)
}
