package tools

import (
	"context"
	"net/http"
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tatnet-ru/tatnet-go/tatnet"
)

type SetAppBranchIn struct {
	AppID  string `json:"app_id"`
	Branch string `json:"branch" jsonschema:"Git branch for future builds and push-triggered deployments"`
}

type SetAppBranchOut struct {
	App  AppView `json:"app"`
	Next string  `json:"next"`
}

func (t *Tools) setAppBranch(ctx context.Context, _ *mcp.CallToolRequest, in SetAppBranchIn) (*mcp.CallToolResult, SetAppBranchOut, error) {
	var out SetAppBranchOut
	var app tatnet.V1App
	// The common transport preserves string-detail 422 responses, which the
	// generated SDK's HTTPValidationError decoder cannot represent.
	if err := t.apiRequest(ctx, http.MethodPatch, "/apps/"+url.PathEscape(in.AppID), tatnet.V1AppUpdate{Branch: &in.Branch}, &app); err != nil {
		return nil, out, err
	}
	out.App = appView(app)
	out.Next = "The running build is unchanged. Call deploy_app to deploy the selected branch."
	return nil, out, nil
}
