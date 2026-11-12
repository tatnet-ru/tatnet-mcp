package tools

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tatnet-ru/tatnet-go/tatnet"
)

type ValkeyTargetIn struct {
	ProjectID string `json:"project_id" jsonschema:"project that owns the Valkey cluster"`
	ClusterID string `json:"cluster_id" jsonschema:"Valkey cluster id"`
}

type UpdateValkeyTLSIn struct {
	ValkeyTargetIn
	TLSMode string `json:"tls_mode" jsonschema:"disabled accepts plaintext only; preferred accepts TLS and plaintext on the same port; required accepts TLS only and disconnects plaintext sessions"`
}

type ValkeyOut struct {
	Cluster map[string]any `json:"cluster" jsonschema:"cluster details including desired tls_mode and parameters_applied; a saved mode is not necessarily applied yet"`
}

type CreateValkeyIn struct {
	ProjectID   string   `json:"project_id"`
	Name        string   `json:"name"`
	PlanID      string   `json:"plan_id"`
	RegionID    string   `json:"region_id,omitempty"`
	VPCID       string   `json:"vpc_id,omitempty"`
	Version     string   `json:"valkey_version,omitempty"`
	Topology    string   `json:"topology,omitempty" jsonschema:"single or ha; defaults to single"`
	TLSMode     string   `json:"tls_mode,omitempty" jsonschema:"disabled, preferred, or required; defaults to required"`
	IPAllowlist []string `json:"ip_allowlist,omitempty" jsonschema:"client CIDRs for a public cluster; the API enforces access and validation"`
}

func valkeyResult(body []byte) (ValkeyOut, error) {
	var out ValkeyOut
	err := json.Unmarshal(body, &out.Cluster)
	return out, err
}

func (t *Tools) updateValkeyTLS(ctx context.Context, in UpdateValkeyTLSIn) (ValkeyOut, error) {
	c, err := t.client(ctx)
	if err != nil {
		return ValkeyOut{}, err
	}
	body, err := json.Marshal(struct {
		TLSMode string `json:"tls_mode"`
	}{in.TLSMode})
	if err != nil {
		return ValkeyOut{}, err
	}
	r, err := c.ValkeyUpdateParamsWithBodyWithResponse(ctx, in.ProjectID, in.ClusterID, "application/json", bytes.NewReader(body))
	if err := check("update Valkey TLS", r, err); err != nil {
		return ValkeyOut{}, err
	}
	return valkeyResult(r.Body)
}

func (t *Tools) registerValkey(s *mcp.Server) {
	add(s, &mcp.Tool{Name: "get_valkey_cluster", Description: "Read a Valkey cluster, its TLS mode, proxy readiness and parameter application status.", Annotations: readOnly("Valkey cluster")},
		func(ctx context.Context, _ *mcp.CallToolRequest, in ValkeyTargetIn) (*mcp.CallToolResult, ValkeyOut, error) {
			c, err := t.client(ctx)
			if err != nil {
				return nil, ValkeyOut{}, err
			}
			r, err := c.ValkeyGetClusterWithResponse(ctx, in.ProjectID, in.ClusterID)
			if err := check("get Valkey cluster", r, err); err != nil {
				return nil, ValkeyOut{}, err
			}
			out, err := valkeyResult(r.Body)
			return nil, out, err
		})
	add(s, &mcp.Tool{Name: "update_valkey_tls_mode", Description: "Change client TLS mode on the existing endpoint port. required disconnects plaintext sessions. Old nodes must be replaced first; the API returns 409 with instructions. Check parameters_applied before reporting completion.", Annotations: destructive("Change Valkey TLS mode")},
		func(ctx context.Context, _ *mcp.CallToolRequest, in UpdateValkeyTLSIn) (*mcp.CallToolResult, ValkeyOut, error) {
			out, err := t.updateValkeyTLS(ctx, in)
			return nil, out, err
		})
	add(s, &mcp.Tool{Name: "create_valkey_cluster", Description: "Create a paid Valkey cluster. TLS defaults to required; preferred accepts both protocols on one port. The response contains the password once: save it securely; it cannot be read again.", Annotations: additive("Create Valkey cluster", false)},
		func(ctx context.Context, _ *mcp.CallToolRequest, in CreateValkeyIn) (*mcp.CallToolResult, ValkeyOut, error) {
			c, err := t.client(ctx)
			if err != nil {
				return nil, ValkeyOut{}, err
			}
			body, err := valkeyCreateBody(in)
			if err != nil {
				return nil, ValkeyOut{}, err
			}
			r, err := c.ValkeyCreateClusterWithBodyWithResponse(ctx, in.ProjectID, "application/json", bytes.NewReader(body))
			if err := check("create Valkey cluster", r, err); err != nil {
				return nil, ValkeyOut{}, err
			}
			out, err := valkeyResult(r.Body)
			return nil, out, err
		})
}

func valkeyCreateBody(in CreateValkeyIn) ([]byte, error) {
	var allowlist *[]string
	if in.IPAllowlist != nil {
		allowlist = &in.IPAllowlist
	}
	optional := func(v string) *string {
		if v == "" {
			return nil
		}
		return &v
	}
	return json.Marshal(struct {
		tatnet.CreateValkeyClusterRequest
		TLSMode string `json:"tls_mode,omitempty"`
	}{tatnet.CreateValkeyClusterRequest{Name: in.Name, PlanId: in.PlanID, ClusterId: optional(in.RegionID), VpcId: optional(in.VPCID), ValkeyVersion: optional(in.Version), Topology: optional(in.Topology), IpAllowlist: allowlist}, in.TLSMode})

}
