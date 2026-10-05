package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type aiModelsInput struct {
	Kind string `json:"kind" jsonschema:"Model category: image or video"`
}
type aiModelsOutput struct {
	Catalog json.RawMessage `json:"catalog"`
}

func (t *Tools) registerAI(s *mcp.Server) {
	add(s, &mcp.Tool{Name: "ai_models", Description: "List public image or video models, capabilities and current retail prices on ai.tatnet.cloud. No generation or spending.", Annotations: readOnly("AI models and prices")}, t.aiModels)
}
func (t *Tools) aiModels(ctx context.Context, _ *mcp.CallToolRequest, in aiModelsInput) (*mcp.CallToolResult, aiModelsOutput, error) {
	var path string
	switch in.Kind {
	case "image":
		path = "images/models"
	case "video":
		path = "videos/models"
	default:
		return nil, aiModelsOutput{}, fmt.Errorf("kind must be image or video")
	}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://ai.tatnet.cloud/v1/"+path, nil)
	if err != nil {
		return nil, aiModelsOutput{}, err
	}
	client := *t.HTTP
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := client.Do(req)
	if err != nil {
		return nil, aiModelsOutput{}, fmt.Errorf("AI catalog unavailable")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || resp.StatusCode != 200 || !json.Valid(raw) {
		return nil, aiModelsOutput{}, fmt.Errorf("AI catalog unavailable (HTTP %d)", resp.StatusCode)
	}
	return nil, aiModelsOutput{Catalog: raw}, nil
}
