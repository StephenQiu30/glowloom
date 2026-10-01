package gltf

import "encoding/json"

type document struct {
	Asset struct {
		Version    string `json:"version"`
		MinVersion string `json:"minVersion"`
	} `json:"asset"`
	ExtensionsUsed     []string                   `json:"extensionsUsed"`
	ExtensionsRequired []string                   `json:"extensionsRequired"`
	Extensions         map[string]json.RawMessage `json:"extensions"`
	Buffers            []struct {
		URI        string `json:"uri"`
		ByteLength int64  `json:"byteLength"`
	} `json:"buffers"`
	BufferViews []bufferView `json:"bufferViews"`
	Accessors   []accessor   `json:"accessors"`
	Images      []struct {
		URI        string `json:"uri"`
		MIMEType   string `json:"mimeType"`
		BufferView *int   `json:"bufferView"`
	} `json:"images"`
	Textures []struct {
		Source     *int                       `json:"source"`
		Sampler    *int                       `json:"sampler"`
		Extensions map[string]json.RawMessage `json:"extensions"`
	} `json:"textures"`
	Samplers  []json.RawMessage `json:"samplers"`
	Materials []json.RawMessage `json:"materials"`
	Meshes    []mesh            `json:"meshes"`
	Nodes     []node            `json:"nodes"`
	Scenes    []struct {
		Nodes []int `json:"nodes"`
	} `json:"scenes"`
	Scene   *int              `json:"scene"`
	Cameras []json.RawMessage `json:"cameras"`
	Skins   []struct {
		Joints              []int `json:"joints"`
		Skeleton            *int  `json:"skeleton"`
		InverseBindMatrices *int  `json:"inverseBindMatrices"`
	} `json:"skins"`
	Animations []animation `json:"animations"`
}
type bufferView struct {
	Buffer     int   `json:"buffer"`
	ByteOffset int64 `json:"byteOffset"`
	ByteLength int64 `json:"byteLength"`
	ByteStride int64 `json:"byteStride"`
	Target     int   `json:"target"`
}
type accessor struct {
	BufferView    *int      `json:"bufferView"`
	ByteOffset    int64     `json:"byteOffset"`
	ComponentType int       `json:"componentType"`
	Normalized    bool      `json:"normalized"`
	Count         int64     `json:"count"`
	Type          string    `json:"type"`
	Min           []float64 `json:"min"`
	Max           []float64 `json:"max"`
	Sparse        *struct {
		Count   int64 `json:"count"`
		Indices struct {
			BufferView    int   `json:"bufferView"`
			ByteOffset    int64 `json:"byteOffset"`
			ComponentType int   `json:"componentType"`
		} `json:"indices"`
		Values struct {
			BufferView int   `json:"bufferView"`
			ByteOffset int64 `json:"byteOffset"`
		} `json:"values"`
	} `json:"sparse"`
}
type mesh struct {
	Primitives []primitive `json:"primitives"`
	Weights    []float64   `json:"weights"`
}
type primitive struct {
	Attributes map[string]int   `json:"attributes"`
	Indices    *int             `json:"indices"`
	Material   *int             `json:"material"`
	Mode       *int             `json:"mode"`
	Targets    []map[string]int `json:"targets"`
}
type node struct {
	Mesh       *int                       `json:"mesh"`
	Skin       *int                       `json:"skin"`
	Camera     *int                       `json:"camera"`
	Children   []int                      `json:"children"`
	Weights    []float64                  `json:"weights"`
	Extensions map[string]json.RawMessage `json:"extensions"`
}
type animation struct {
	Samplers []struct {
		Input         int    `json:"input"`
		Output        int    `json:"output"`
		Interpolation string `json:"interpolation"`
	} `json:"samplers"`
	Channels []struct {
		Sampler int `json:"sampler"`
		Target  struct {
			Node *int   `json:"node"`
			Path string `json:"path"`
		} `json:"target"`
	} `json:"channels"`
}

func index(value, length int) bool              { return value >= 0 && value < length }
func optionalIndex(value *int, length int) bool { return value == nil || index(*value, length) }
