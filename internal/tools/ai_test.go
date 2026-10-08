package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type aiTransport func(*http.Request) (*http.Response, error)

func (f aiTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestAICatalogDoesNotForwardCredentials(t *testing.T) {
	tools := New("https://platform.invalid")
	calls := 0
	tools.HTTP.Transport = aiTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://ai.tatnet.cloud/v1/images/models" || len(r.Header) != 0 {
			t.Fatal(r.URL, r.Header)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"object":"list","data":[]}`)), Header: make(http.Header)}, nil
	})
	_, out, err := tools.aiModels(context.Background(), nil, aiModelsInput{Kind: "image"})
	if err != nil || len(out.Catalog) == 0 {
		t.Fatal(out, err)
	}
	if _, _, err := tools.aiModels(context.Background(), nil, aiModelsInput{Kind: "../../../internal"}); err == nil {
		t.Fatal("invalid kind accepted")
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}

func TestAIChatCatalogIsPublic(t *testing.T) {
	for _, kind := range []string{"chat", "text"} {
		t.Run(kind, func(t *testing.T) {
			tools := New("https://platform.invalid")
			tools.HTTP.Transport = aiTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://ai.tatnet.cloud/v1/models" || len(r.Header) != 0 {
					t.Fatal(r.URL, r.Header)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"glm-5.2"}]}`)), Header: make(http.Header)}, nil
			})
			_, out, err := tools.aiModels(context.Background(), nil, aiModelsInput{Kind: kind})
			raw, _ := json.Marshal(out.Catalog)
			if err != nil || !strings.Contains(string(raw), "glm-5.2") {
				t.Fatal(out, err)
			}
		})
	}
}

func TestAICatalogMCPOutputSchema(t *testing.T) {
	for _, kind := range []string{"chat", "text", "image", "video"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			tools := New("https://platform.invalid")
			tools.HTTP.Transport = aiTransport(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"object":"list","data":[{"id":"example","capabilities":{"tools":true},"context_window":32000}]}`)), Header: make(http.Header)}, nil
			})
			server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
			tools.registerAI(server)
			ct, st := mcp.NewInMemoryTransports()
			ss, err := server.Connect(ctx, st, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer ss.Close()
			client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil)
			cs, err := client.Connect(ctx, ct, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer cs.Close()
			result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "ai_models", Arguments: map[string]any{"kind": kind}})
			if err != nil {
				t.Fatal(err)
			}
			if result.IsError {
				t.Fatalf("catalog rejected by MCP schema: %+v", result)
			}
			raw, err := json.Marshal(result.StructuredContent)
			if err != nil || !strings.Contains(string(raw), `"context_window":32000`) || !strings.Contains(string(raw), `"tools":true`) {
				t.Fatalf("catalog fields lost: %s, %v", raw, err)
			}
		})
	}
}
