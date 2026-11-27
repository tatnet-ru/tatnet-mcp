package tools

import (
	"context"
	"net/http"
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type AppTrustedIPsIn struct {
	ProjectID string `json:"project_id"`
	AppID     string `json:"app_id"`
}

type SetAppTrustedIPsIn struct {
	ProjectID  string   `json:"project_id"`
	AppID      string   `json:"app_id"`
	TrustedIPs []string `json:"trusted_ips" jsonschema:"IPv4 addresses and CIDRs. An empty array makes the app public."`
}

type AppTrustedIPsOut struct {
	TrustedIPs []string `json:"trusted_ips"`
}

func appTrustedIPsPath(projectID, appID string) string {
	return "/projects/" + url.PathEscape(projectID) + "/apps/" + url.PathEscape(appID) + "/trusted-ips"
}

func (t *Tools) getAppTrustedIPs(ctx context.Context, _ *mcp.CallToolRequest, in AppTrustedIPsIn) (*mcp.CallToolResult, AppTrustedIPsOut, error) {
	var out AppTrustedIPsOut
	err := t.apiRequest(ctx, http.MethodGet, appTrustedIPsPath(in.ProjectID, in.AppID), nil, &out)
	return nil, out, err
}

func (t *Tools) setAppTrustedIPs(ctx context.Context, _ *mcp.CallToolRequest, in SetAppTrustedIPsIn) (*mcp.CallToolResult, AppTrustedIPsOut, error) {
	var out AppTrustedIPsOut
	body := AppTrustedIPsOut{TrustedIPs: in.TrustedIPs}
	err := t.apiRequest(ctx, http.MethodPut, appTrustedIPsPath(in.ProjectID, in.AppID), body, &out)
	return nil, out, err
}
