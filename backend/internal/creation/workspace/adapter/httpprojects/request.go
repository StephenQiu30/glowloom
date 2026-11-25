package httpprojects

import "github.com/google/uuid"

// CreateProjectRequest contains only user-editable creation specifications.
type CreateProjectRequest struct {
	StyleKey      *string    `json:"style_key" maxLength:"50" extensions:"x-nullable"`
	Name          string     `json:"name" binding:"required" minLength:"1" maxLength:"50"`
	Description   string     `json:"description"`
	AspectRatio   string     `json:"aspect_ratio" binding:"required" enums:"9:16,16:9,1:1"`
	StyleType     string     `json:"style_type" enums:"realistic,stylized"`
	StyleSubtype  string     `json:"style_subtype,omitempty" enums:"anime_jp,guofeng_xianxia,cartoon_3d,manhwa"`
	StylePresetID *uuid.UUID `json:"style_preset_id,omitempty"`
}
