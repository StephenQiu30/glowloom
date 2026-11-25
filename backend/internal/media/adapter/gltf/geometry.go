package gltf

import "strings"

func (d document) validateGeometry(values [][]float64) error {
	primitives := 0
	quantized := false
	for _, name := range d.ExtensionsUsed {
		quantized = quantized || name == "KHR_mesh_quantization"
	}
	for _, mesh := range d.Meshes {
		targets := -1
		for _, p := range mesh.Primitives {
			primitives++
			if primitives > maxPrimitives || len(p.Targets) > 64 || targets != -1 && targets != len(p.Targets) || !optionalIndex(p.Indices, len(d.Accessors)) || !optionalIndex(p.Material, len(d.Materials)) {
				return invalid("primitive budget or references")
			}
			targets = len(p.Targets)
			positionID, ok := p.Attributes["POSITION"]
			if !ok || !index(positionID, len(d.Accessors)) {
				return invalid("missing positions")
			}
			position := d.Accessors[positionID]
			if position.Type != "VEC3" || len(position.Min) != 3 || len(position.Max) != 3 || position.ComponentType != 5126 && !quantized {
				return invalid("position format")
			}
			for name, id := range p.Attributes {
				if !index(id, len(d.Accessors)) {
					return invalid("attribute reference")
				}
				a := d.Accessors[id]
				if a.Count != position.Count || a.ComponentType == 5125 {
					return invalid("attribute count or component")
				}
				switch {
				case name == "POSITION", name == "NORMAL":
					if a.Type != "VEC3" {
						return invalid("vector attribute")
					}
				case name == "TANGENT", strings.HasPrefix(name, "JOINTS_"), strings.HasPrefix(name, "WEIGHTS_"):
					if a.Type != "VEC4" {
						return invalid("four component attribute")
					}
				case strings.HasPrefix(name, "TEXCOORD_"):
					if a.Type != "VEC2" {
						return invalid("texture coordinate attribute")
					}
				case strings.HasPrefix(name, "COLOR_"):
					if a.Type != "VEC3" && a.Type != "VEC4" {
						return invalid("color attribute")
					}
				case strings.HasPrefix(name, "_"):
				default:
					return invalid("unknown attribute semantic")
				}
				if a.BufferView != nil && (d.BufferViews[*a.BufferView].ByteOffset+a.ByteOffset)%4 != 0 {
					return invalid("vertex alignment")
				}
			}
			count := position.Count
			if p.Indices != nil {
				a := d.Accessors[*p.Indices]
				if a.Type != "SCALAR" || a.Normalized || a.ComponentType != 5121 && a.ComponentType != 5123 && a.ComponentType != 5125 || a.BufferView != nil && d.BufferViews[*a.BufferView].ByteStride != 0 {
					return invalid("index format")
				}
				for _, value := range values[*p.Indices] {
					if value >= float64(position.Count) {
						return invalid("index outside positions")
					}
				}
				count = a.Count
			}
			mode := 4
			if p.Mode != nil {
				mode = *p.Mode
			}
			if mode < 0 || mode > 6 || mode == 1 && count%2 != 0 || mode >= 2 && mode <= 3 && count < 2 || mode == 4 && count%3 != 0 || mode >= 5 && count < 3 {
				return invalid("primitive topology")
			}
			for _, target := range p.Targets {
				for name, id := range target {
					if name != "POSITION" && name != "NORMAL" && name != "TANGENT" || !index(id, len(d.Accessors)) {
						return invalid("morph target reference")
					}
					a := d.Accessors[id]
					if a.Type != "VEC3" || a.Count != position.Count || a.ComponentType != 5126 && !quantized {
						return invalid("morph target shape")
					}
				}
			}
		}
		if len(mesh.Weights) != 0 && len(mesh.Weights) != targets {
			return invalid("mesh morph weights")
		}
	}
	return nil
}
