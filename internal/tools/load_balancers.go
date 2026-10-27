package tools

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/url"
)

type CreateLoadBalancerIn struct {
	ProjectID    string         `json:"project_id" jsonschema:"Project UUID"`
	Name         string         `json:"name" jsonschema:"Load balancer name"`
	VPCID        string         `json:"vpc_id" jsonschema:"Customer VPC UUID; determines the region"`
	FloatingIPID string         `json:"floating_ip_id,omitempty" jsonschema:"Optional reserved public IP UUID in the same account and region; retained after LB deletion"`
	NodeCount    int            `json:"node_count" jsonschema:"Number of LB nodes, from 1 to 3"`
	Config       map[string]any `json:"config,omitempty" jsonschema:"Optional declarative target_groups and listeners document; validated by the public API"`
}
type GetLoadBalancerIn struct {
	ProjectID      string `json:"project_id"`
	LoadBalancerID string `json:"load_balancer_id"`
}

func (t *Tools) registerLoadBalancers(s *mcp.Server) {
	add(s, &mcp.Tool{Name: "create_load_balancer", Description: "Create a billable internet-facing load balancer. Provide floating_ip_id to reuse an available account-owned public IP that survives LB deletion. Requires load_balancer:create and, when reserving an IP, floating_ip:write. Creation is asynchronous; the returned status reflects regional readiness.", Annotations: additive("Create load balancer", false)}, t.createLoadBalancer)
	add(s, &mcp.Tool{Name: "get_load_balancer", Description: "Get a load balancer's VIP, reserved IP allocation, config and regional status. Requires load_balancer:read.", Annotations: readOnly("Get load balancer")}, t.getLoadBalancer)
}
func (t *Tools) createLoadBalancer(ctx context.Context, _ *mcp.CallToolRequest, in CreateLoadBalancerIn) (*mcp.CallToolResult, ObjectOut, error) {
	body := map[string]any{"name": in.Name, "vpc_id": in.VPCID, "node_count": in.NodeCount}
	if in.FloatingIPID != "" {
		body["floating_ip_id"] = in.FloatingIPID
	}
	if in.Config != nil {
		body["config"] = in.Config
	}
	return t.objectRequest(ctx, http.MethodPost, "/projects/"+url.PathEscape(in.ProjectID)+"/load-balancers", body)
}
func (t *Tools) getLoadBalancer(ctx context.Context, _ *mcp.CallToolRequest, in GetLoadBalancerIn) (*mcp.CallToolResult, ObjectOut, error) {
	return t.objectRequest(ctx, http.MethodGet, "/projects/"+url.PathEscape(in.ProjectID)+"/load-balancers/"+url.PathEscape(in.LoadBalancerID), nil)
}
