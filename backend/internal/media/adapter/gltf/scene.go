package gltf

import (
	"encoding/json"
	"strings"
)

func (d document) validateScene(values [][]float64) error {
	if len(d.Scenes) == 0 || len(d.Scenes) > 128 || !optionalIndex(d.Scene, len(d.Scenes)) {
		return invalid("scene reference")
	}
	parents := make([]int, len(d.Nodes))
	var drawWork int64
	for _, node := range d.Nodes {
		if !optionalIndex(node.Mesh, len(d.Meshes)) || !optionalIndex(node.Camera, len(d.Cameras)) || !optionalIndex(node.Skin, len(d.Skins)) || node.Skin != nil && node.Mesh == nil {
			return invalid("node reference")
		}
		for _, child := range node.Children {
			if !index(child, len(d.Nodes)) {
				return invalid("child reference")
			}
			parents[child]++
			if parents[child] > 1 {
				return invalid("multiple node parents")
			}
		}
		instances, err := d.instanceCount(node)
		if err != nil {
			return err
		}
		if node.Mesh != nil {
			mesh := d.Meshes[*node.Mesh]
			if len(node.Weights) != 0 && len(node.Weights) != len(mesh.Primitives[0].Targets) {
				return invalid("node morph weights")
			}
			for _, p := range mesh.Primitives {
				drawWork += d.Accessors[p.Attributes["POSITION"]].Count * instances
				if drawWork > maxDrawVertices {
					return invalid("draw vertex budget")
				}
				if node.Skin != nil {
					for name, id := range p.Attributes {
						if strings.HasPrefix(name, "JOINTS_") {
							for _, joint := range values[id] {
								if joint < 0 || joint >= float64(len(d.Skins[*node.Skin].Joints)) || joint != float64(int64(joint)) {
									return invalid("skin joint index")
								}
							}
						}
					}
				}
			}
		}
	}
	marks := make([]byte, len(d.Nodes))
	var visit func(int, int) error
	visit = func(id, depth int) error {
		if depth > maxDepth || marks[id] == 1 {
			return invalid("node cycle or depth")
		}
		if marks[id] == 2 {
			return nil
		}
		marks[id] = 1
		for _, child := range d.Nodes[id].Children {
			if err := visit(child, depth+1); err != nil {
				return err
			}
		}
		marks[id] = 2
		return nil
	}
	for id := range d.Nodes {
		if err := visit(id, 0); err != nil {
			return err
		}
	}
	for _, scene := range d.Scenes {
		seen := make(map[int]bool)
		for _, id := range scene.Nodes {
			if !index(id, len(d.Nodes)) || parents[id] != 0 || seen[id] {
				return invalid("scene root reference")
			}
			seen[id] = true
		}
	}
	for _, skin := range d.Skins {
		if len(skin.Joints) > 512 || !optionalIndex(skin.Skeleton, len(d.Nodes)) || !optionalIndex(skin.InverseBindMatrices, len(d.Accessors)) {
			return invalid("skin reference or budget")
		}
		seen := make(map[int]bool)
		for _, id := range skin.Joints {
			if !index(id, len(d.Nodes)) || seen[id] {
				return invalid("skin joints")
			}
			seen[id] = true
		}
		if skin.InverseBindMatrices != nil {
			a := d.Accessors[*skin.InverseBindMatrices]
			if a.Type != "MAT4" || a.ComponentType != 5126 || a.Count < int64(len(skin.Joints)) {
				return invalid("inverse bind matrices")
			}
		}
	}
	return d.validateAnimations(values)
}

func (d document) instanceCount(node node) (int64, error) {
	body := node.Extensions["EXT_mesh_gpu_instancing"]
	if body == nil {
		return 1, nil
	}
	if node.Mesh == nil || node.Skin != nil {
		return 0, invalid("instance mesh")
	}
	var instance struct {
		Attributes map[string]int `json:"attributes"`
	}
	if json.Unmarshal(body, &instance) != nil {
		return 0, invalid("instance attributes")
	}
	var count int64
	for name, id := range instance.Attributes {
		if !index(id, len(d.Accessors)) {
			return 0, invalid("instance accessor")
		}
		a := d.Accessors[id]
		if a.Count > maxNodes || count != 0 && count != a.Count {
			return 0, invalid("instance count budget")
		}
		count = a.Count
		switch {
		case name == "TRANSLATION" || name == "SCALE":
			if a.Type != "VEC3" || a.ComponentType != 5126 {
				return 0, invalid("instance vector")
			}
		case name == "ROTATION":
			if a.Type != "VEC4" || a.ComponentType != 5126 && !a.Normalized {
				return 0, invalid("instance rotation")
			}
		case !strings.HasPrefix(name, "_"):
			return 0, invalid("instance semantic")
		}
	}
	if count < 1 {
		return 0, invalid("instance count")
	}
	return count, nil
}

func (d document) validateAnimations(values [][]float64) error {
	for _, animation := range d.Animations {
		if len(animation.Channels) > 4096 || len(animation.Samplers) > 4096 {
			return invalid("animation budget")
		}
		for _, sampler := range animation.Samplers {
			if !index(sampler.Input, len(d.Accessors)) || !index(sampler.Output, len(d.Accessors)) {
				return invalid("animation accessor")
			}
			input := d.Accessors[sampler.Input]
			if input.Type != "SCALAR" || input.ComponentType != 5126 {
				return invalid("animation timestamps")
			}
			previous := float64(-1)
			for _, value := range values[sampler.Input] {
				if value <= previous {
					return invalid("animation timestamp order")
				}
				previous = value
			}
		}
		seen := make(map[string]bool)
		for _, channel := range animation.Channels {
			if !index(channel.Sampler, len(animation.Samplers)) || channel.Target.Node == nil || !index(*channel.Target.Node, len(d.Nodes)) {
				return invalid("animation target")
			}
			key := string(rune(*channel.Target.Node)) + ":" + channel.Target.Path
			if seen[key] {
				return invalid("duplicate animation target")
			}
			seen[key] = true
			sampler := animation.Samplers[channel.Sampler]
			input, output := d.Accessors[sampler.Input], d.Accessors[sampler.Output]
			multiplier := int64(1)
			if sampler.Interpolation == "CUBICSPLINE" {
				multiplier = 3
			}
			if output.ComponentType != 5126 {
				return invalid("animation output component")
			}
			switch channel.Target.Path {
			case "translation", "scale":
				if output.Type != "VEC3" || output.Count != input.Count*multiplier {
					return invalid("animation vector output")
				}
			case "rotation":
				if output.Type != "VEC4" || output.Count != input.Count*multiplier {
					return invalid("animation rotation output")
				}
			case "weights":
				node := d.Nodes[*channel.Target.Node]
				if node.Mesh == nil || len(d.Meshes[*node.Mesh].Primitives) == 0 || output.Type != "SCALAR" || output.Count != input.Count*multiplier*int64(len(d.Meshes[*node.Mesh].Primitives[0].Targets)) {
					return invalid("animation morph output")
				}
			default:
				return invalid("animation path")
			}
		}
	}
	return nil
}
