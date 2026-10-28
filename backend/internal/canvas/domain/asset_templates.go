package domain

import (
	"encoding/json"

	"github.com/google/uuid"
)

// SystemAssetTemplate is an immutable system recipe, never a user's canvas.
type SystemAssetTemplate struct {
	Key           string `json:"key"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	IsTemplate    bool   `json:"is_template"`
	NodeCount     int    `json:"node_count"`
	RequiresModel bool   `json:"requires_model"`
}

// SystemAssetTemplates returns fresh metadata; versioned recipe keys are immutable.
func SystemAssetTemplates(kind string) ([]SystemAssetTemplate, error) {
	if kind != "" && kind != "canvas" && kind != "short_drama" {
		return nil, ErrInvalidCommand
	}
	catalog := []SystemAssetTemplate{
		{Key: "text-to-image-v1", Kind: "canvas", Name: "文生图工作流", Description: "从可编辑创作说明连接到图片生成节点，选择可用模型后再手动生成。", IsTemplate: true, NodeCount: 2, RequiresModel: true},
		{Key: "storyboard-three-shots-v1", Kind: "short_drama", Name: "三镜头分镜", Description: "为开场、冲突、转折预填三组可编辑分镜与图片生成节点，选择模型后手动生成。", IsTemplate: true, NodeCount: 6, RequiresModel: true},
	}
	result := make([]SystemAssetTemplate, 0, len(catalog))
	for _, item := range catalog {
		if kind == "" || item.Kind == kind {
			result = append(result, item)
		}
	}
	return result, nil
}

// SystemAssetTemplateByKey resolves a server-owned recipe without caller content.
func SystemAssetTemplateByKey(key string) (SystemAssetTemplate, error) {
	catalog, _ := SystemAssetTemplates("")
	for _, item := range catalog {
		if item.Key == key {
			return item, nil
		}
	}
	return SystemAssetTemplate{}, ErrInvalidCommand
}

// ApplySystemAssetTemplate seeds only a new empty document with editable intent.
// IDs belong to the new document; no template entity or media identity is reused.
func ApplySystemAssetTemplate(base Document, key string) (OperationOutcome, error) {
	template, err := SystemAssetTemplateByKey(key)
	if err != nil || base.ID == uuid.Nil || base.ProjectID == uuid.Nil || base.Revision != 1 || len(base.Nodes) != 0 || len(base.Edges) != 0 {
		return OperationOutcome{}, ErrInvalidCommand
	}
	kind, err := ParseDocumentKind(base.Kind)
	if err != nil || kind != template.Kind {
		return OperationOutcome{}, ErrInvalidCommand
	}
	prompts := []string{"描述画面主体、动作、场景、构图与光线，例如：清晨的咖啡馆，一杯热咖啡置于木桌中央，窗外柔和晨光，温暖写实风格。"}
	titles := []string{"创作说明"}
	if template.Kind == "short_drama" {
		titles = []string{"镜头一 · 开场", "镜头二 · 冲突", "镜头三 · 转折"}
		prompts = []string{
			"开场：雨夜的街角，一位年轻人攥着一封旧信停在书店门口。交代主角、目标与环境，远景，冷色雨光。请按你的剧本修改人物与场景。",
			"冲突：主角推开书店门，与熟悉的身影意外相遇。保留上一镜的人物和服装设定，中景，门外冷光与店内暖光对比。请补充关键动作与表情。",
			"转折：对方递来一把旧钥匙，主角看清钥匙上的刻字，决定留下。保留前两镜的角色与地点，手部近景，柔和侧光。请改写悬念和情绪落点。",
		}
	}
	operations := make([]Operation, 0, len(prompts)*3)
	width, height := 400.0, 280.0
	binding := json.RawMessage(`{"schemaVersion":1,"fromPortId":"text","toPortId":"prompt","purpose":"PROMPT","order":0,"binding":{"kind":"NODE_VALUE"}}`)
	for i, prompt := range prompts {
		textID := uuid.NewSHA1(base.ID, []byte("template/"+key+"/text/"+titles[i]))
		imageID := uuid.NewSHA1(base.ID, []byte("template/"+key+"/image/"+titles[i]))
		text := Node{ID: textID, Title: titles[i], NodeType: "text", NodeAction: "resource", Config: NodeConfig{Text: &prompt}, X: 0, Y: float64(i) * 360, Width: &width, Height: &height, ZIndex: int32(i * 2)}
		image := Node{ID: imageID, Title: titles[i] + " · 图片", NodeType: "image", NodeAction: "resource", Config: NodeConfig{Editor: &EditorConfig{SchemaVersion: 1, Parameters: &EditorParameters{Mode: "GENERATE"}}}, X: 560, Y: float64(i) * 360, Width: &width, Height: &height, ZIndex: int32(i*2 + 1)}
		edge := Edge{ID: uuid.NewSHA1(base.ID, []byte("template/"+key+"/edge/"+titles[i])), EdgeType: "annotation", SourceNodeID: textID, TargetNodeID: imageID, Binding: append(json.RawMessage(nil), binding...)}
		operations = append(operations, Operation{Type: "create_node", Node: &text}, Operation{Type: "create_node", Node: &image}, Operation{Type: "connect", Edge: &edge})
	}
	return ApplyOperations(base, operations)
}
