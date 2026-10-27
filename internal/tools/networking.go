package tools

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/url"
	"strconv"
)

type RenameVpcIn struct {
	VPCID string `json:"vpc_id" jsonschema:"VPC UUID"`
	Name  string `json:"name" jsonschema:"New display name; subnet and OVN identifiers stay unchanged"`
}

type DisableNatGatewayIn struct {
	VPCID         string `json:"vpc_id" jsonschema:"VPC UUID"`
	RetainIP      bool   `json:"retain_ip" jsonschema:"Keep the public IP allocated and billable after disabling NAT; wait until the floating IP is available before reusing it"`
	ExpectedFIPID string `json:"expected_fip_id,omitempty" jsonschema:"Optional expected floating IP UUID; returns 409 if the gateway has been replaced"`
}

type EnableNatGatewayIn struct {
	VPCID string `json:"vpc_id" jsonschema:"VPC UUID"`
	FIPID string `json:"fip_id,omitempty" jsonschema:"Available floating IP UUID in the same account and region; omit to allocate a new billable IP"`
}

func (t *Tools) registerNetworking(s *mcp.Server) {
	add(s, &mcp.Tool{Name: "rename_vpc", Description: "Rename a private network's display label without changing its subnet, ports, NAT or OVN identifiers. Requires vpc:update.", Annotations: additive("Rename VPC", true)}, t.renameVpc)
	add(s, &mcp.Tool{Name: "disable_nat_gateway", Description: "Disable a VPC's NAT gateway asynchronously. Interrupts outgoing internet access through this gateway. Set retain_ip=true to keep the IP allocated and billable for reuse; false releases it after the region removes SNAT. Requires vpc:write. Returns the floating IP ID and detaching status.", Annotations: destructive("Disable NAT gateway")}, t.disableNatGateway)
	add(s, &mcp.Tool{Name: "enable_nat_gateway", Description: "Enable a VPC's NAT gateway asynchronously for outgoing internet access. Reuse a retained, available floating IP via fip_id, or omit it to allocate a new billable address. NAT gateway and IP charges apply. Requires vpc:write. Returns the address, floating IP ID and attachment status.", Annotations: additive("Enable NAT gateway", false)}, t.enableNatGateway)
}

func (t *Tools) enableNatGateway(ctx context.Context, _ *mcp.CallToolRequest, in EnableNatGatewayIn) (*mcp.CallToolResult, ObjectOut, error) {
	body := map[string]string{}
	if in.FIPID != "" {
		body["fip_id"] = in.FIPID
	}
	return t.objectRequest(ctx, http.MethodPost, "/vpcs/"+url.PathEscape(in.VPCID)+"/nat-gateway", body)
}

func (t *Tools) disableNatGateway(ctx context.Context, _ *mcp.CallToolRequest, in DisableNatGatewayIn) (*mcp.CallToolResult, ObjectOut, error) {
	query := url.Values{"retain_ip": {strconv.FormatBool(in.RetainIP)}}
	if in.ExpectedFIPID != "" {
		query.Set("expected_fip_id", in.ExpectedFIPID)
	}
	return t.objectRequest(ctx, http.MethodDelete, "/vpcs/"+url.PathEscape(in.VPCID)+"/nat-gateway?"+query.Encode(), nil)
}
func (t *Tools) renameVpc(ctx context.Context, _ *mcp.CallToolRequest, in RenameVpcIn) (*mcp.CallToolResult, ObjectOut, error) {
	return t.objectRequest(ctx, http.MethodPatch, "/vpcs/"+url.PathEscape(in.VPCID), map[string]string{"name": in.Name})
}
