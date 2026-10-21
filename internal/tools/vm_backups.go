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

type DeleteVMBackupIn struct {
	ProjectID string `json:"project_id" jsonschema:"Project UUID"`
	VMID      string `json:"vm_id" jsonschema:"VM UUID"`
	BackupID  string `json:"backup_id" jsonschema:"Recovery point UUID; deletion is permanent and the API preserves protection and dependency checks"`
}

func (t *Tools) registerVMBackups(s *mcp.Server) {
	add(s, &mcp.Tool{Name: "delete_vm_backup", Description: "Permanently delete a VM backup through the public API. Also permits explicitly deleting abandoned legacy pending placeholders older than seven days with no dispatch or stored result. Recent captures, protected versions and required chain ancestors remain guarded by the API. Requires vm:update.", Annotations: destructive("Delete VM backup")}, t.deleteVMBackup)
	add(s, &mcp.Tool{Name: "get_vm_backup_policy", Description: "Get VM daily backup schedule and actual stored bytes. Requires vm:read.", Annotations: readOnly("VM backup policy")}, t.getVMBackupPolicy)
	add(s, &mcp.Tool{Name: "set_vm_backup_policy", Description: "Configure daily VM backups and retention. Storage is billed by compressed bytes held over time. Requires vm:update.", Annotations: additive("Set VM backup policy", true)}, t.setVMBackupPolicy)
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

func (t *Tools) deleteVMBackup(ctx context.Context, _ *mcp.CallToolRequest, in DeleteVMBackupIn) (*mcp.CallToolResult, ObjectOut, error) {
	path := "/projects/" + url.PathEscape(in.ProjectID) + "/vms/" + url.PathEscape(in.VMID) + "/backups/" + url.PathEscape(in.BackupID)
	return t.objectRequest(ctx, http.MethodDelete, path, nil)
}
