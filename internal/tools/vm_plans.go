package tools

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tatnet-ru/tatnet-go/tatnet"
)

type VMPlansIn struct {
	ClusterID string `json:"cluster_id,omitempty" jsonschema:"region UUID to filter by compatible hardware; omit for all regions"`
	Role      string `json:"role,omitempty" jsonschema:"product: vm (default), k8s, db or lb"`
}

type VMPlansOut struct {
	Plans []tatnet.VMFormPlanResponse `json:"plans"`
}

func (t *Tools) listVMPlans(ctx context.Context, _ *mcp.CallToolRequest, in VMPlansIn) (*mcp.CallToolResult, VMPlansOut, error) {
	var out VMPlansOut
	c, err := t.client(ctx)
	if err != nil {
		return nil, out, err
	}
	params := &tatnet.VmsListPlansParams{}
	if in.Role != "" {
		params.Role = ptr(tatnet.VmsListPlansParamsRole(in.Role))
	}
	if in.ClusterID != "" {
		id, err := uuid.Parse(in.ClusterID)
		if err != nil {
			return nil, out, fmt.Errorf("invalid cluster_id UUID: %w", err)
		}
		params.ClusterId = &id
	}
	r, err := c.VmsListPlansWithResponse(ctx, params)
	if err := check("list VM plans", r, err); err != nil {
		return nil, out, err
	}
	if r.JSON200 == nil || r.JSON200.Plans == nil {
		return nil, out, fmt.Errorf("VM plan catalog has no plans array")
	}
	out.Plans = r.JSON200.Plans
	return nil, out, nil
}

func (t *Tools) registerVMPlans(s *mcp.Server) {
	add(s, &mcp.Tool{
		Name:        "list_vm_plans",
		Description: "Find available VM plan UUIDs before creating a VM. Returns vCPU, memory in MiB (8 GiB = 8192), disk limits, CPU family/tags and RUB prices. Filter by cluster_id for region compatibility and role (vm by default; k8s for managed Kubernetes). Requires vm:read. Use the returned id as vm_plan_id; never guess UUIDs or request dashboard credentials.",
		Annotations: readOnly("VM plans"),
	}, t.listVMPlans)
}
