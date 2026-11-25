package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/execution/gateway/domain"
)

func (c *Client) sendEmbedding(ctx context.Context, d application.Dispatch, key string) (domain.Receipt, error) {
	var out domain.Receipt
	var in application.EmbeddingInput
	if d.Route.BaseURL != domain.EmbeddingBaseURL || d.Route.UpstreamModel != domain.EmbeddingModel || json.Unmarshal(d.Call.Raw, &in) != nil {
		return out, application.ErrNotSent
	}
	instruction := "Instruction:Compress the text into one word.\nQuery:"
	switch in.Purpose {
	case "query":
		instruction = "Target_modality: text.\nInstruction:为这个句子生成表示以用于检索相关创作资料\nQuery:"
	case "document":
	default:
		return out, application.ErrNotSent
	}
	body, err := json.Marshal(map[string]any{"model": domain.EmbeddingModel, "instructions": instruction, "dimensions": domain.EmbeddingDimensions, "encoding_format": "float", "input": []map[string]string{{"type": "text", "text": in.Text}}})
	if err != nil {
		return out, application.ErrNotSent
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, domain.EmbeddingBaseURL+"/embeddings/multimodal", bytes.NewReader(body))
	if err != nil {
		return out, application.ErrNotSent
	}
	resp, err := (fenced{client: c, route: d.Route, key: key}).Do(req)
	if err != nil {
		return out, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return out, domain.ErrUnknown
	}
	const limit = 128 << 10
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	var reply struct {
		ID    string `json:"id"`
		Model string `json:"model"`
		Data  struct {
			Embedding json.RawMessage `json:"embedding"`
		} `json:"data"`
		Usage struct {
			PromptTokens int64 `json:"prompt_tokens"`
			TotalTokens  int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err != nil || len(data) > limit || json.Unmarshal(data, &reply) != nil || reply.Model != domain.EmbeddingModel || reply.ID == "" || len(reply.ID) > 256 || reply.Usage.PromptTokens <= 0 || reply.Usage.PromptTokens != reply.Usage.TotalTokens || reply.Usage.PromptTokens > d.Call.Model.MaxInput {
		return out, domain.ErrUnknown
	}
	vector, err := domain.EmbeddingVector(reply.Data.Embedding)
	if err != nil {
		return out, domain.ErrUnknown
	}
	out.Body, err = json.Marshal(vector)
	if err != nil {
		return out, domain.ErrUnknown
	}
	out.UpstreamID = reply.ID
	out.Usage.Input = reply.Usage.PromptTokens
	out.PurchaseMicros, err = d.Route.Purchase.Cost(out.Usage)
	return out, err
}
