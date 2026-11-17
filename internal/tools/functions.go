package tools

import (
	"context"
	"net/http"
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type functionRegionsIn struct {
	ProjectID string `json:"project_id" jsonschema:"Project ID"`
}

type functionDeployIn struct {
	ProjectID  string `json:"project_id" jsonschema:"Project ID"`
	FunctionID string `json:"function_id" jsonschema:"Function ID"`
	Region     string `json:"region,omitempty" jsonschema:"Execution region for the first deployment"`
}

func (t *Tools) registerFunctions(s *mcp.Server) {
	add(s, &mcp.Tool{
		Name: "list_function_regions", Description: "List available serverless function deployment regions in a project.",
		Annotations: readOnly("List function regions"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in functionRegionsIn) (*mcp.CallToolResult, ObjectOut, error) {
		return t.objectRequest(ctx, http.MethodGet, "/projects/"+url.PathEscape(in.ProjectID)+"/function-regions", nil)
	})
	add(s, &mcp.Tool{
		Name: "deploy_function", Description: "Build and deploy a serverless function. The first deployment sets its execution region.",
		Annotations: additive("Deploy function", false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in functionDeployIn) (*mcp.CallToolResult, ObjectOut, error) {
		var body any
		if in.Region != "" {
			body = map[string]string{"region": in.Region}
		}
		path := "/projects/" + url.PathEscape(in.ProjectID) + "/functions/" + url.PathEscape(in.FunctionID) + "/deploy"
		return t.objectRequest(ctx, http.MethodPost, path, body)
	})
}
