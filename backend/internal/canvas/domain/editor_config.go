package domain

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

// EditorConfig is editable canvas content, never execution authority or preview grants.
type EditorConfig struct {
	SchemaVersion int                        `json:"schemaVersion"`
	Parameters    *EditorParameters          `json:"parameters,omitempty"`
	Metadata      map[string]json.RawMessage `json:"metadata,omitempty" swaggertype:"object"`
	Document      json.RawMessage            `json:"document,omitempty" swaggertype:"object"`
	Files         []EditorFileReference      `json:"files,omitempty"`
}

// EditorParameters holds editable intent, without creating a generation task.
type EditorParameters struct {
	Mode                   string                     `json:"mode,omitempty"`
	Agent                  *EditorAgent               `json:"agent,omitempty"`
	ModelSelection         *EditorModelSelection      `json:"modelSelection,omitempty"`
	ModelParameters        map[string]json.RawMessage `json:"modelParameters,omitempty" swaggertype:"object"`
	ModelParameterSchemaID string                     `json:"modelParameterSchemaId,omitempty"`
	Values                 map[string]json.RawMessage `json:"values,omitempty" swaggertype:"object"`
}

// EditorAgent stores the selected agent settings.
type EditorAgent struct {
	Profile     string `json:"profile"`
	Goal        string `json:"goal"`
	MaxRounds   int    `json:"maxRounds"`
	MaxTools    int    `json:"maxTools"`
	Parallelism int    `json:"parallelism"`
}

// EditorModelSelection records the selected provider connection and model.
type EditorModelSelection struct {
	ConnectionID string `json:"connectionId"`
	ModelID      string `json:"modelId"`
}

// EditorFileReference pins a project asset behind a stable canvas reference key.
type EditorFileReference struct {
	ReferenceKey string    `json:"referenceKey"`
	FileID       uuid.UUID `json:"fileId"`
	Version      string    `json:"version"`
	Purpose      string    `json:"purpose"`
}

func validEditor(c *EditorConfig) bool {
	if c == nil {
		return true
	}
	body, err := json.Marshal(c)
	if err != nil || len(body) > 1<<20 || c.SchemaVersion != 1 || len(c.Files) > 64 {
		return false
	}
	if p := c.Parameters; p != nil {
		if p.Mode != "" && p.Mode != "MANUAL" && p.Mode != "GENERATE" && p.Mode != "AGENT" && p.Mode != "PROCESS" {
			return false
		}
		if !boundedSettings(p.ModelParameters) || !boundedSettings(p.Values) {
			return false
		}
		if p.ModelSelection != nil && (len(p.ModelSelection.ConnectionID) > 256 || len(p.ModelSelection.ModelID) > 256) {
			return false
		}
		if len(p.ModelParameterSchemaID) > 256 {
			return false
		}
		if p.Agent != nil && (len(p.Agent.Goal) > 40000 || len(p.Agent.Profile) > 256 || p.Agent.MaxRounds < 0 || p.Agent.MaxRounds > 1000 || p.Agent.MaxTools < 0 || p.Agent.MaxTools > 1000 || p.Agent.Parallelism < 0 || p.Agent.Parallelism > 100) {
			return false
		}
	}
	for key, value := range c.Metadata {
		switch key {
		case "colorTags":
			var tags []string
			if json.Unmarshal(value, &tags) != nil || tags == nil || len(tags) > 5 {
				return false
			}
			seen := map[string]bool{}
			for _, tag := range tags {
				switch tag {
				case "cyan", "blue", "purple", "orange", "yellow":
				default:
					return false
				}
				if seen[tag] {
					return false
				}
				seen[tag] = true
			}
		case "textBackground":
			var color string
			if json.Unmarshal(value, &color) != nil {
				return false
			}
			switch color {
			case "default", "cyan", "blue", "purple", "orange", "yellow":
			default:
				return false
			}
		case "referenceLabels":
			var labels map[string]struct {
				Kind    string `json:"kind"`
				Ordinal int64  `json:"ordinal"`
			}
			if json.Unmarshal(value, &labels) != nil || len(labels) > 10000 {
				return false
			}
			used := map[string]bool{}
			for id, label := range labels {
				if _, err := uuid.Parse(id); err != nil {
					return false
				}
				if label.Kind != "image" && label.Kind != "video" && label.Kind != "audio" && label.Kind != "text" {
					return false
				}
				if label.Ordinal < 1 || label.Ordinal > 9007199254740991 {
					return false
				}
				key := label.Kind + strconv.FormatInt(label.Ordinal, 10)
				if used[key] {
					return false
				}
				used[key] = true
			}

		case "fontSize", "count", "textCount":
			var number float64
			if json.Unmarshal(value, &number) != nil || number < 0 || number > 1000 {
				return false
			}
		case "freeResize", "interactive":
			var flag bool
			if json.Unmarshal(value, &flag) != nil {
				return false
			}
		case "prompt", "composerContent", "generationMode", "generationType", "model", "reasoningEffort", "size", "quality", "background", "seconds", "vquality", "generateAudio", "watermark", "videoMode", "audioVoice", "audioFormat", "audioSpeed", "audioInstructions":
			var text string
			if json.Unmarshal(value, &text) != nil || !utf8.ValidString(text) || utf8.RuneCountInString(text) > 10000 {
				return false
			}
		default:
			return false
		}
	}
	if len(c.Document) > 0 && !validRichText(c.Document, &prompt.Draft{}) {
		return false
	}
	keys := map[string]bool{}
	for _, ref := range c.Files {
		version, err := strconv.ParseInt(ref.Version, 10, 64)
		if ref.FileID == uuid.Nil || ref.ReferenceKey == "" || len(ref.ReferenceKey) > 256 || keys[ref.ReferenceKey] || err != nil || version < 1 || strconv.FormatInt(version, 10) != ref.Version || (ref.Purpose != "CONTENT" && ref.Purpose != "LAST_FRAME") {
			return false
		}
		keys[ref.ReferenceKey] = true
	}
	return true
}
func boundedSettings(values map[string]json.RawMessage) bool {
	if len(values) > 128 {
		return false
	}
	var walk func(any, int) bool
	walk = func(v any, depth int) bool {
		if depth > 8 {
			return false
		}
		switch x := v.(type) {
		case map[string]any:
			if len(x) > 128 {
				return false
			}
			for k, v := range x {
				if len(k) > 256 || !walk(v, depth+1) {
					return false
				}
			}
		case []any:
			if len(x) > 256 {
				return false
			}
			for _, v := range x {
				if !walk(v, depth+1) {
					return false
				}
			}
		case string:
			if len(x) > 40000 || strings.HasPrefix(x, "data:") || strings.HasPrefix(x, "blob:") {
				return false
			}
		}
		return true
	}
	for key, raw := range values {
		var value any
		if len(key) > 256 || len(raw) > 65536 || json.Unmarshal(raw, &value) != nil || !walk(value, 0) {
			return false
		}
	}
	return true
}
func editorNodeType(node Node) string {
	switch node.NodeType {
	case "image_generation":
		return "image"
	case "video_generation":
		return "video"
	case "generation":
		return "config"
	}
	return node.NodeType
}
func validEditorFiles(node Node) bool {
	if node.Config.Editor == nil {
		return true
	}
	var primary *EditorFileReference
	for i, ref := range node.Config.Editor.Files {
		if editorNodeType(node) != "image" && editorNodeType(node) != "video" && editorNodeType(node) != "audio" {
			return false
		}
		if ref.Purpose == "LAST_FRAME" && editorNodeType(node) != "video" {
			return false
		}
		if ref.Purpose == "CONTENT" && primary == nil {
			primary = &node.Config.Editor.Files[i]
		}
	}
	if primary == nil {
		return node.RefID == nil
	}
	return node.RefType == "media_asset" && node.RefID != nil && *node.RefID == primary.FileID
}

// UnmarshalJSON rejects unsupported nested editor data instead of silently losing it.
func (c *EditorConfig) UnmarshalJSON(body []byte) error {
	type wire EditorConfig
	var value wire
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	*c = EditorConfig(value)
	return nil
}
