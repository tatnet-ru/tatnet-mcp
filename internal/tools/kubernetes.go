package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tatnet-ru/tatnet-mcp/internal/authn"
)

type KubernetesSettingsIn struct {
	ProjectID     string `json:"project_id" jsonschema:"Project UUID"`
	ClusterID     string `json:"cluster_id" jsonschema:"Kubernetes cluster UUID"`
	TLSMinVersion string `json:"tls_min_version" jsonschema:"Minimum Kubernetes API TLS version: VersionTLS12 or VersionTLS13. VersionTLS13 requires TLS 1.3 support in all API clients"`
}

type KubernetesSettingsOut struct {
	ID            string `json:"id"`
	TLSMinVersion string `json:"tls_min_version" jsonschema:"Desired minimum TLS version; applied asynchronously to control-plane nodes"`
}

func (t *Tools) registerKubernetes(s *mcp.Server) {
	add(s, &mcp.Tool{
		Name:        "set_kubernetes_tls",
		Description: "Set the minimum TLS version of a managed Kubernetes cluster's API server (TLS 1.2 or 1.3). Requires k8s_cluster:update permission. The cluster must be ready with no version upgrade in progress. Configuration applies to control-plane nodes asynchronously, one at a time; a single-node control plane may briefly interrupt API connections.",
		Annotations: additive("Kubernetes API TLS settings", true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in KubernetesSettingsIn) (*mcp.CallToolResult, KubernetesSettingsOut, error) {
		p, ok := authn.FromContext(ctx)
		if !ok {
			return nil, KubernetesSettingsOut{}, fmt.Errorf("not authenticated")
		}
		out, err := t.setKubernetesTLS(ctx, p, in)
		return nil, out, err
	})
}

func (t *Tools) setKubernetesTLS(ctx context.Context, p authn.Principal, in KubernetesSettingsIn) (KubernetesSettingsOut, error) {
	var out KubernetesSettingsOut
	// Path identifiers cannot become URL segments or query parameters.
	if _, err := uuid.Parse(in.ProjectID); err != nil {
		return out, fmt.Errorf("invalid project_id")
	}
	if _, err := uuid.Parse(in.ClusterID); err != nil {
		return out, fmt.Errorf("invalid cluster_id")
	}
	payload, err := json.Marshal(map[string]string{"tls_min_version": in.TLSMinVersion})
	if err != nil {
		return out, err
	}
	endpoint := strings.TrimRight(t.APIBase, "/") + "/projects/" + url.PathEscape(in.ProjectID) + "/kubernetes-clusters/" + url.PathEscape(in.ClusterID) + "/settings"
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, endpoint, bytes.NewReader(payload))
	if err != nil {
		return out, err
	}
	req.Header.Set("Content-Type", "application/json")
	if err := p.Editor(t.InternalSecret)(ctx, req); err != nil {
		return out, err
	}
	resp, err := t.HTTP.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return out, err
	}
	if resp.StatusCode != http.StatusOK {
		return out, apiError("set Kubernetes TLS", resp, body)
	}
	err = json.Unmarshal(body, &out)
	return out, err
}
