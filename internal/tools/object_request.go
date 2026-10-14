package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tatnet-ru/tatnet-mcp/internal/authn"
	"io"
	"net/http"
	"strings"
)

type ObjectOut struct {
	Data map[string]any `json:"data"`
}

func (t *Tools) objectRequest(ctx context.Context, method, path string, body any) (*mcp.CallToolResult, ObjectOut, error) {
	p, ok := authn.FromContext(ctx)
	if !ok {
		return nil, ObjectOut{}, fmt.Errorf("not authenticated")
	}
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return nil, ObjectOut{}, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(t.APIBase, "/")+path, bytes.NewReader(data))
	if err != nil {
		return nil, ObjectOut{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if err = p.Editor(t.InternalSecret)(ctx, req); err != nil {
		return nil, ObjectOut{}, err
	}
	res, err := t.HTTP.Do(req)
	if err != nil {
		return nil, ObjectOut{}, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, ObjectOut{}, err
	}
	if res.StatusCode >= 400 {
		return nil, ObjectOut{}, fmt.Errorf("API returned %d: %s", res.StatusCode, raw)
	}
	out := ObjectOut{}
	err = json.Unmarshal(raw, &out.Data)
	return nil, out, err
}
