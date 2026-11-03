package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tatnet-ru/tatnet-go/tatnet"
)

type AppVolumeIn struct {
	ProjectID string `json:"project_id"`
	VolumeID  string `json:"volume_id"`
	AppID     string `json:"app_id"`
	MountPath string `json:"mount_path" jsonschema:"/data or a directory below /data"`
}

type DetachVolumeIn struct {
	AppID     string `json:"app_id"`
	ProjectID string `json:"project_id"`
	VolumeID  string `json:"volume_id"`
}
type ListVolumesIn struct {
	ProjectID string `json:"project_id"`
}
type VolumesOut struct {
	Volumes []map[string]any `json:"volumes"`
}
type VolumeOut struct {
	Volume map[string]any `json:"volume,omitempty"`
	Note   string         `json:"note"`
}

func (t *Tools) registerVolumes(s *mcp.Server) {
	add(s, &mcp.Tool{Name: "list_volumes", Description: "List block volumes and their VM or app reservations in a project.", Annotations: readOnly("List block volumes")},
		func(ctx context.Context, _ *mcp.CallToolRequest, in ListVolumesIn) (*mcp.CallToolResult, VolumesOut, error) {
			out := VolumesOut{Volumes: []map[string]any{}}
			c, err := t.client(ctx)
			if err != nil {
				return nil, out, err
			}
			limit, offset := 100, 0
			for {
				r, err := c.VolumesListVolumesWithResponse(ctx, in.ProjectID, &tatnet.VolumesListVolumesParams{Limit: &limit, Offset: &offset})
				if err := check("list volumes", r, err); err != nil {
					return nil, out, err
				}
				var page struct {
					Data []map[string]any `json:"data"`
				}
				if err := json.Unmarshal(r.Body, &page); err != nil {
					return nil, out, err
				}
				out.Volumes = append(out.Volumes, page.Data...)
				if len(page.Data) < limit {
					break
				}
				offset += limit
			}
			return nil, out, nil
		})
	add(s, &mcp.Tool{Name: "attach_app_volume", Description: "Attach an existing preformatted ext4 volume to one Firecracker backend replica in the same project and region. Restarts the app with brief downtime. Requires regional app-volume support; does not format or delete data.", Annotations: additive("Attach app volume", false)},
		func(ctx context.Context, _ *mcp.CallToolRequest, in AppVolumeIn) (*mcp.CallToolResult, VolumeOut, error) {
			out := VolumeOut{Note: "Attachment is asynchronous. Read the app deployment state and volume attachment before reporting success."}
			id, err := uuid.Parse(in.AppID)
			if err != nil {
				return nil, out, fmt.Errorf("invalid app_id: %w", err)
			}
			c, err := t.client(ctx)
			if err != nil {
				return nil, out, err
			}
			payload, _ := json.Marshal(map[string]any{"app_id": id.String(), "mount_path": in.MountPath})
			r, err := c.VolumesAttachVolumeWithBodyWithResponse(ctx, in.ProjectID, in.VolumeID, "application/json", bytes.NewReader(payload))
			if err := check("attach app volume", r, err); err != nil {
				return nil, out, err
			}
			if err := json.Unmarshal(r.Body, &out.Volume); err != nil {
				return nil, out, err
			}
			return nil, out, nil
		})
	add(s, &mcp.Tool{Name: "detach_app_volume", Description: "Detach an app's block volume. Restarts the app and retains the disk and data. Reservation remains until storage confirms release. Does not force-detach a live VM disk.", Annotations: destructive("Detach app volume")},
		func(ctx context.Context, _ *mcp.CallToolRequest, in DetachVolumeIn) (*mcp.CallToolResult, VolumeOut, error) {
			out := VolumeOut{Note: "Detach is asynchronous. The disk remains billed until explicitly deleted; read it again to confirm release."}
			c, err := t.client(ctx)
			if err != nil {
				return nil, out, err
			}
			current, err := c.VolumesGetVolumeWithResponse(ctx, in.ProjectID, in.VolumeID)
			if err := check("get app volume", current, err); err != nil {
				return nil, out, err
			}
			var before struct {
				Attachment *struct {
					AppID *string `json:"app_id"`
				} `json:"attachment"`
			}
			if err := json.Unmarshal(current.Body, &before); err != nil {
				return nil, out, err
			}
			if before.Attachment == nil || before.Attachment.AppID == nil || *before.Attachment.AppID != in.AppID {
				return nil, out, fmt.Errorf("volume is not reserved by an app")
			}
			payload, _ := json.Marshal(map[string]any{"force": false, "app_id": in.AppID})
			r, err := c.VolumesDetachVolumeWithBodyWithResponse(ctx, in.ProjectID, in.VolumeID, "application/json", bytes.NewReader(payload))
			if err := check("detach app volume", r, err); err != nil {
				return nil, out, err
			}
			if err := json.Unmarshal(r.Body, &out.Volume); err != nil {
				return nil, out, err
			}
			return nil, out, nil
		})
}
