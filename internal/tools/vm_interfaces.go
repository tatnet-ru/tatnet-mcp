package tools

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/url"
)

type AttachVMInterfaceIn struct {
	ProjectID string `json:"project_id"`
	VMID      string `json:"vm_id"`
	VPCID     string `json:"vpc_id"`
}
type DetachVMInterfaceIn struct {
	ProjectID   string `json:"project_id"`
	VMID        string `json:"vm_id"`
	InterfaceID string `json:"interface_id"`
}

func (t *Tools) attachVMInterface(ctx context.Context, _ *mcp.CallToolRequest, in AttachVMInterfaceIn) (*mcp.CallToolResult, ObjectOut, error) {
	return t.objectRequest(ctx, http.MethodPost, "/projects/"+url.PathEscape(in.ProjectID)+"/vms/"+url.PathEscape(in.VMID)+"/interfaces", map[string]string{"vpc_id": in.VPCID})
}
func (t *Tools) detachVMInterface(ctx context.Context, _ *mcp.CallToolRequest, in DetachVMInterfaceIn) (*mcp.CallToolResult, ObjectOut, error) {
	return t.objectRequest(ctx, http.MethodDelete, "/projects/"+url.PathEscape(in.ProjectID)+"/vms/"+url.PathEscape(in.VMID)+"/interfaces/"+url.PathEscape(in.InterfaceID), nil)
}
func (t *Tools) registerVMInterfaces(s *mcp.Server) {
	add(s, &mcp.Tool{Name: "attach_vm_interface", Description: "Attach a separate VPC interface to a stopped VM. VPC must belong to the same account and region. The primary route and floating IP are preserved. Requires vm:update and vpc:write.", Annotations: additive("Attach VPC interface", false)}, t.attachVMInterface)
	add(s, &mcp.Tool{Name: "detach_vm_interface", Description: "Detach a secondary VPC interface from a stopped VM. Primary and floating-IP interfaces are protected. VM loses connectivity to that VPC. Requires vm:update and vpc:write.", Annotations: destructive("Detach VPC interface")}, t.detachVMInterface)
}
