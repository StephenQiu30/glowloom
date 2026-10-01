package gltf

import (
	"encoding/json"
	"math"
	"strings"
)

func supportedExtension(name string) bool {
	switch name {
	case "KHR_lights_punctual", "KHR_materials_anisotropy", "KHR_materials_clearcoat", "KHR_materials_dispersion", "KHR_materials_emissive_strength", "KHR_materials_ior", "KHR_materials_specular", "KHR_materials_transmission", "KHR_materials_iridescence", "KHR_materials_unlit", "KHR_materials_volume", "KHR_materials_sheen", "KHR_mesh_quantization", "KHR_texture_transform", "EXT_materials_bump", "EXT_mesh_gpu_instancing", "EXT_texture_webp", "EXT_texture_avif":
		return true
	default:
		return false
	}
}

func (v *Validator) validateExtensions(value any, scope string, root map[string]any, depth int) error {
	if depth > maxDepth {
		return invalid("extension depth")
	}
	switch x := value.(type) {
	case map[string]any:
		if root == nil {
			root = x
			for _, field := range []string{"extensionsUsed", "extensionsRequired"} {
				if names, ok := x[field].([]any); ok {
					for _, name := range names {
						if !supportedExtension(name.(string)) {
							return invalid("unsupported extension")
						}
					}
				}
			}
			if required, ok := x["extensionsRequired"].([]any); ok {
				used, _ := x["extensionsUsed"].([]any)
				for _, requiredName := range required {
					found := false
					for _, usedName := range used {
						found = found || usedName == requiredName
					}
					if !found {
						return invalid("undeclared required extension")
					}
				}
			}
		}
		if ext, ok := x["extensions"].(map[string]any); ok {
			for name, payload := range ext {
				if !supportedExtension(name) {
					return invalid("unsupported extension payload")
				}
				declared := false
				if used, ok := root["extensionsUsed"].([]any); ok {
					for _, item := range used {
						declared = declared || item == name
					}
				}
				if !declared {
					return invalid("undeclared extension")
				}
				if err := v.validateExtension(name, payload, scope, root); err != nil {
					return err
				}
			}
		}
		if strings.HasSuffix(scope, "Texture") || scope == "textureInfo" {
			if _, ok := x["index"]; ok {
				if !jsonIndex(x["index"], collectionLength(root, "textures")) {
					return invalid("texture info reference")
				}
				if coord, ok := x["texCoord"]; ok && !jsonIndex(coord, 8) {
					return invalid("texture coordinate budget")
				}
			}
		}
		for key, child := range x {
			if key == "extras" {
				continue
			} // Metadata is never interpreted by the loader.
			if key == "extensions" {
				if ext, ok := child.(map[string]any); ok {
					for _, payload := range ext {
						if err := v.validateExtensions(payload, "extension", root, depth+1); err != nil {
							return err
						}
					}
				}
				continue
			}
			if err := v.validateExtensions(child, key, root, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range x {
			if err := v.validateExtensions(child, scope, root, depth+1); err != nil {
				return err
			}
		}
	case json.Number:
		n, err := x.Float64()
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || math.Abs(n) > 1e12 {
			return invalid("numeric budget")
		}
	}
	return nil
}

func (v *Validator) validateExtension(name string, payload any, scope string, root map[string]any) error {
	m, ok := payload.(map[string]any)
	if !ok {
		return invalid("extension object")
	}
	var file string
	switch {
	case strings.HasPrefix(name, "KHR_materials_") && scope == "materials":
		file = "material." + name + ".schema.json"
	case name == "KHR_lights_punctual" && scope == "root":
		file = "glTF.KHR_lights_punctual.schema.json"
	case name == "KHR_lights_punctual" && scope == "nodes":
		file = "node.KHR_lights_punctual.schema.json"
		ext, _ := root["extensions"].(map[string]any)
		lights, _ := ext[name].(map[string]any)
		if !jsonIndex(m["light"], collectionLength(lights, "lights")) {
			return invalid("light reference")
		}
	case name == "KHR_texture_transform" && (strings.HasSuffix(scope, "Texture") || scope == "textureInfo"):
		file = "textureInfo.KHR_texture_transform.schema.json"
		if coord, exists := m["texCoord"]; exists && !jsonIndex(coord, 8) {
			return invalid("texture transform coordinate budget")
		}
	case name == "KHR_mesh_quantization":
		if len(m) != 0 {
			return invalid("quantization extension payload")
		}
		return nil
	case name == "EXT_texture_webp" || name == "EXT_texture_avif":
		if scope != "textures" || !jsonIndex(m["source"], collectionLength(root, "images")) {
			return invalid("extension texture source")
		}
		return nil
	case name == "EXT_mesh_gpu_instancing":
		if scope != "nodes" {
			return invalid("instancing placement")
		}
		attrs, ok := m["attributes"].(map[string]any)
		if !ok || len(attrs) == 0 || len(attrs) > 16 {
			return invalid("instance attributes")
		}
		for _, value := range attrs {
			if !jsonIndex(value, collectionLength(root, "accessors")) {
				return invalid("instance accessor")
			}
		}
		return nil
	case name == "EXT_materials_bump":
		if scope != "materials" {
			return invalid("bump material placement")
		}
		if factor, exists := m["bumpFactor"]; exists {
			n, ok := factor.(json.Number)
			if !ok {
				return invalid("bump factor")
			}
			f, err := n.Float64()
			if err != nil || f < 0 {
				return invalid("bump factor")
			}
		}
		if texture, exists := m["bumpTexture"]; exists {
			obj, ok := texture.(map[string]any)
			if !ok || !jsonIndex(obj["index"], collectionLength(root, "textures")) {
				return invalid("bump texture")
			}
		}
		return nil
	default:
		return invalid("extension placement")
	}
	if schema := v.extensions[file]; schema == nil || schema.Validate(payload) != nil {
		return invalid("extension schema")
	}
	if lights, ok := m["lights"].([]any); ok && len(lights) > 128 {
		return invalid("light budget")
	}
	return nil
}

func collectionLength(root map[string]any, name string) int {
	items, _ := root[name].([]any)
	return len(items)
}
func jsonIndex(value any, length int) bool {
	n, ok := value.(json.Number)
	if !ok {
		return false
	}
	id, err := n.Int64()
	return err == nil && id >= 0 && id < int64(length)
}
