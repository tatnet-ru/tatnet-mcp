package tools

import (
	"context"
	"net/http"
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tatnet-ru/tatnet-go/tatnet"
)

type CreateVMIn struct {
	ProjectID string            `json:"project_id" jsonschema:"Project UUID; the API key must permit vm:create and account billing management"`
	VM        tatnet.V1VMCreate `json:"vm" jsonschema:"VM settings, including optional daily backup_policy saved atomically with the VM; creating a VM incurs the selected plan charge"`
}

func (t *Tools) registerVMs(s *mcp.Server) {
	add(s, &mcp.Tool{Name: "create_vm", Description: "Create a paid virtual machine through the public API. Select an existing image, plan and cluster, and optionally enable daily backups in backup_policy. Backup storage is billed separately by actual compressed usage. Requires vm:create and billing management; do not retry with a different VM name after an uncertain result.", Annotations: additive("Create virtual machine", false)}, t.createVM)
	add(s, &mcp.Tool{Name: "list_vm_backups", Description: "List VM recovery points, compressed sizes, automatic expiry and legacy immutable protection deadlines. Requires vm:read.", Annotations: readOnly("VM recovery points")}, t.listVMBackups)
}
func (t *Tools) createVM(ctx context.Context, _ *mcp.CallToolRequest, in CreateVMIn) (*mcp.CallToolResult, ObjectOut, error) {
	return t.objectRequest(ctx, http.MethodPost, "/projects/"+url.PathEscape(in.ProjectID)+"/vms", in.VM)
}
func (t *Tools) listVMBackups(ctx context.Context, _ *mcp.CallToolRequest, in VMBackupPolicyIn) (*mcp.CallToolResult, ObjectOut, error) {
	return t.objectRequest(ctx, http.MethodGet, "/projects/"+url.PathEscape(in.ProjectID)+"/vms/"+url.PathEscape(in.VMID)+"/backups", nil)
}
