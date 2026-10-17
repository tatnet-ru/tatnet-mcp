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
)

type S3RegionIn struct {
	Region string `json:"region,omitempty" jsonschema:"S3 region, for example spb-1. Omit to use the API default"`
}

type S3RegionsOut struct {
	Regions []struct {
		Region   string `json:"region"`
		Endpoint string `json:"endpoint"`
		Enabled  bool   `json:"enabled"`
	} `json:"regions"`
}

type S3BucketIn struct {
	Name       string `json:"name" jsonschema:"Globally unique bucket name"`
	Region     string `json:"region,omitempty" jsonschema:"S3 region; omitted uses the API default"`
	IsPublic   bool   `json:"is_public,omitempty" jsonschema:"Allow unsigned object reads; defaults to private"`
	Versioning bool   `json:"versioning,omitempty"`
}

type S3BucketOut struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Region        string  `json:"region"`
	Status        string  `json:"status"`
	IsPublic      bool    `json:"is_public"`
	PublicBaseURL *string `json:"public_base_url,omitempty"`
}

type S3ConnectionOut struct {
	Endpoint             string `json:"endpoint"`
	Region               string `json:"region"`
	PathStyleExample     string `json:"path_style_example"`
	VirtualHostedExample string `json:"virtual_hosted_example"`
}

// Keep the existing SDK's authenticated transport while adding the new regional
// endpoints. All policy checks and admission remain in /v1.
func (t *Tools) s3Request(ctx context.Context, method, path string, input, output any) error {
	c, err := t.streamClient(ctx)
	if err != nil {
		return err
	}
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.Server, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, edit := range c.RequestEditors {
		if err = edit(ctx, req); err != nil {
			return err
		}
	}
	resp, err := t.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("S3 request failed: %w", err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return apiError("S3 request", resp, data)
	}
	if err = json.Unmarshal(data, output); err != nil {
		return fmt.Errorf("invalid S3 response: %w", err)
	}
	return nil
}

func (t *Tools) registerS3(s *mcp.Server) {
	add(s, &mcp.Tool{Name: "list_s3_regions", Description: "List S3 regions and public endpoints. enabled=false means the region is closed for new buckets.", Annotations: readOnly("S3 regions")}, func(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, S3RegionsOut, error) {
		var out S3RegionsOut
		err := t.s3Request(ctx, http.MethodGet, "/object-storage/regions", nil, &out)
		return nil, out, err
	})
	add(s, &mcp.Tool{Name: "get_s3_connection_info", Description: "Get the S3 endpoint and connection examples for a region. Credentials are managed separately.", Annotations: readOnly("S3 connection")}, func(ctx context.Context, _ *mcp.CallToolRequest, in S3RegionIn) (*mcp.CallToolResult, S3ConnectionOut, error) {
		var out S3ConnectionOut
		c, err := t.client(ctx)
		if err != nil {
			return nil, out, err
		}
		r, err := c.ObjectStorageGetConnectionInfoWithResponse(ctx, func(_ context.Context, r *http.Request) error {
			if in.Region != "" {
				q := r.URL.Query()
				q.Set("region", in.Region)
				r.URL.RawQuery = q.Encode()
			}
			return nil
		})
		if err = check("get S3 connection", r, err); err != nil {
			return nil, out, err
		}
		err = json.Unmarshal(r.Body, &out)
		return nil, out, err
	})
	add(s, &mcp.Tool{Name: "create_s3_bucket", Description: "Create a paid S3 bucket in the selected region. Names are globally unique. Private by default. Existing bucket names produce a conflict; Use an available region.", Annotations: additive("Create S3 bucket", false)}, func(ctx context.Context, _ *mcp.CallToolRequest, in S3BucketIn) (*mcp.CallToolResult, S3BucketOut, error) {
		var out S3BucketOut
		err := t.s3Request(ctx, http.MethodPost, "/object-storage/buckets", in, &out)
		return nil, out, err
	})
}
