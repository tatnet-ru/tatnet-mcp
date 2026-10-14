package tools

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/url"
)

type RenameVpcIn struct {
	VPCID string `json:"vpc_id" jsonschema:"VPC UUID"`
	Name  string `json:"name" jsonschema:"New display name; subnet and OVN identifiers stay unchanged"`
}

func (t *Tools) registerNetworking(s *mcp.Server) {
	add(s, &mcp.Tool{Name: "rename_vpc", Description: "Rename a private network's display label without changing its subnet, ports, NAT or OVN identifiers. Requires vpc:update.", Annotations: additive("Rename VPC", true)}, t.renameVpc)
}
func (t *Tools) renameVpc(ctx context.Context, _ *mcp.CallToolRequest, in RenameVpcIn) (*mcp.CallToolResult, ObjectOut, error) {
	return t.objectRequest(ctx, http.MethodPatch, "/vpcs/"+url.PathEscape(in.VPCID), map[string]string{"name": in.Name})
}
