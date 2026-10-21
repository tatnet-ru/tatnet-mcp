package tools

import (
	"context"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net/http"
	"net/url"
)

type VMBackupPolicyIn struct {
	ProjectID string `json:"project_id" jsonschema:"Project UUID"`
	VMID      string `json:"vm_id" jsonschema:"VM UUID"`
}
type SetVMBackupPolicyIn struct {
	ProjectID     string `json:"project_id" jsonschema:"Project UUID"`
	VMID          string `json:"vm_id" jsonschema:"VM UUID"`
	Enabled       bool   `json:"enabled" jsonschema:"Enable daily automatic backups"`
	HourUTC       int    `json:"hour_utc" jsonschema:"Daily capture hour UTC, 0 through 23"`
	RetentionDays int    `json:"retention_days" jsonschema:"Keep future automatic copies for 1 through 365 days; manual copies are never expired"`
}

func (t *Tools) registerVMBackups(s *mcp.Server) {
	add(s, &mcp.Tool{Name: "get_vm_backup_policy", Description: "Get VM daily backup schedule and actual stored bytes. Requires vm:read.", Annotations: readOnly("VM backup policy")}, t.getVMBackupPolicy)
	add(s, &mcp.Tool{Name: "set_vm_backup_policy", Description: "Configure daily VM backups and retention. Storage is billed by compressed bytes held over time. Existing prepaid slots are removed with proration before storage billing starts. Requires vm:update.", Annotations: additive("Set VM backup policy", true)}, t.setVMBackupPolicy)
}
func backupPolicyPath(project, id string) string {
	return "/projects/" + url.PathEscape(project) + "/vms/" + url.PathEscape(id) + "/backup-policy"
}
func (t *Tools) getVMBackupPolicy(ctx context.Context, _ *mcp.CallToolRequest, in VMBackupPolicyIn) (*mcp.CallToolResult, ObjectOut, error) {
	return t.objectRequest(ctx, http.MethodGet, backupPolicyPath(in.ProjectID, in.VMID), nil)
}
func (t *Tools) setVMBackupPolicy(ctx context.Context, _ *mcp.CallToolRequest, in SetVMBackupPolicyIn) (*mcp.CallToolResult, ObjectOut, error) {
	return t.objectRequest(ctx, http.MethodPut, backupPolicyPath(in.ProjectID, in.VMID), map[string]any{"enabled": in.Enabled, "hour_utc": in.HourUTC, "retention_days": in.RetentionDays})
}
