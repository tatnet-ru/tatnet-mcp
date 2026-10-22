package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tatnet-ru/tatnet-mcp/internal/authn"
)

type ObjectOut struct {
	Data map[string]any `json:"data"`
}

// apiRequest is the authenticated JSON transport for public /v1 endpoints
// not yet wrapped by the SDK. It uses the same principal editor as SDK calls.
func (t *Tools) apiRequest(ctx context.Context, method, path string, body, output any) error {
	p, ok := authn.FromContext(ctx)
	if !ok {
		return fmt.Errorf("not authenticated")
	}
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(t.APIBase, "/")+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if err = p.Editor(t.InternalSecret)(ctx, req); err != nil {
		return err
	}
	res, err := t.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("API request failed: %w", err)
	}
	defer res.Body.Close()
	const maxResponseBytes = 4 << 20
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxResponseBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > maxResponseBytes {
		return fmt.Errorf("API response exceeds %d bytes", maxResponseBytes)
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return apiError("API request", res, raw)
	}
	// Successful deletion may have no representation. Do not turn it into a
	// decoding error that could cause the caller to repeat the operation.
	if res.StatusCode == http.StatusNoContent {
		return nil
	}
	if err = json.Unmarshal(raw, output); err != nil {
		return fmt.Errorf("invalid API response: %w", err)
	}
	return nil
}

func (t *Tools) objectRequest(ctx context.Context, method, path string, body any) (*mcp.CallToolResult, ObjectOut, error) {
	out := ObjectOut{Data: map[string]any{}}
	err := t.apiRequest(ctx, method, path, body, &out.Data)
	return nil, out, err
}
