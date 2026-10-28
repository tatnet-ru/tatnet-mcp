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

type RetainLoadBalancerIPIn struct {
	ProjectID      string `json:"project_id" jsonschema:"Project UUID"`
	LoadBalancerID string `json:"load_balancer_id" jsonschema:"Existing load balancer UUID"`
	ExpectedVIP    string `json:"expected_vip" jsonschema:"Exact current public IPv4; a mismatch is rejected without choosing another address"`
}

func (t *Tools) registerLoadBalancers(s *mcp.Server) {
	add(s, &mcp.Tool{Name: "retain_load_balancer_ip", Description: "Retain an existing load balancer's current public IP without changing its VIP, recreating the LB or interrupting traffic. Requires load_balancer:update and floating_ip:write. Idempotent with expected_vip; returns the allocation UUID. Existing LB IP billing stays included until its subscription is released, then the retained IP is billed separately. For Kubernetes, persist the returned floating_ip_id in tatnet/lb-floating-ip-id and Helm/GitOps values before recreating the Service.", Annotations: additive("Retain current load balancer IP", true)}, t.retainLoadBalancerIP)

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

func (t *Tools) retainLoadBalancerIP(ctx context.Context, _ *mcp.CallToolRequest, in RetainLoadBalancerIPIn) (*mcp.CallToolResult, ObjectOut, error) {
	return t.objectRequest(ctx, http.MethodPost, "/projects/"+url.PathEscape(in.ProjectID)+"/load-balancers/"+url.PathEscape(in.LoadBalancerID)+"/retain-ip", map[string]any{"expected_vip": in.ExpectedVIP})
}
