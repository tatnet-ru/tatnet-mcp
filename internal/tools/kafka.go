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

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tatnet-ru/tatnet-mcp/internal/authn"
)

type KafkaProjectIn struct {
	ProjectID string `json:"project_id" jsonschema:"Project UUID"`
}
type KafkaClusterIn struct {
	ProjectID string `json:"project_id" jsonschema:"Project UUID"`
	ClusterID string `json:"cluster_id" jsonschema:"Kafka cluster UUID"`
}
type KafkaCreateIn struct {
	ProjectID        string   `json:"project_id"`
	Name             string   `json:"name"`
	RegionID         string   `json:"region_id"`
	PlanID           string   `json:"plan_id"`
	Topology         string   `json:"topology" jsonschema:"dev (one VM) or ha (three controllers and three brokers)"`
	ControllerPlanID string   `json:"controller_plan_id,omitempty"`
	VPCID            string   `json:"vpc_id,omitempty"`
	Version          string   `json:"kafka_version,omitempty"`
	RetentionHours   int      `json:"retention_hours,omitempty" jsonschema:"Message retention in hours (1..8760); default 168"`
	Allowlist        []string `json:"ip_allowlist" jsonschema:"Public IPv4 CIDR allowlist; empty denies public clients"`
}
type KafkaAllowlistIn struct {
	ProjectID string   `json:"project_id"`
	ClusterID string   `json:"cluster_id"`
	Allowlist []string `json:"ip_allowlist"`
}
type KafkaOut struct {
	Data map[string]any `json:"data"`
}

func kafkaPath(project, id string) string {
	p := "/projects/" + url.PathEscape(project) + "/kafka-clusters"
	if id != "" {
		p += "/" + url.PathEscape(id)
	}
	return p
}
func (t *Tools) kafkaRequest(ctx context.Context, method, path string, body any) (*mcp.CallToolResult, KafkaOut, error) {
	p, ok := authn.FromContext(ctx)
	if !ok {
		return nil, KafkaOut{}, fmt.Errorf("not authenticated")
	}
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return nil, KafkaOut{}, err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(t.APIBase, "/")+path, bytes.NewReader(data))
	if err != nil {
		return nil, KafkaOut{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if err = p.Editor(t.InternalSecret)(ctx, req); err != nil {
		return nil, KafkaOut{}, err
	}
	res, err := t.HTTP.Do(req)
	if err != nil {
		return nil, KafkaOut{}, err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, KafkaOut{}, err
	}
	if res.StatusCode >= 400 {
		return nil, KafkaOut{}, fmt.Errorf("Kafka API returned %d: %s", res.StatusCode, raw)
	}
	out := KafkaOut{}
	err = json.Unmarshal(raw, &out.Data)
	return nil, out, err
}
func (t *Tools) registerKafka(s *mcp.Server) {
	add(s, &mcp.Tool{Name: "kafka_regions", Description: "Available managed Kafka regions, Dev/HA topology and VM plans.", Annotations: readOnly("Kafka regions")}, func(ctx context.Context, _ *mcp.CallToolRequest, _ WhoamiIn) (*mcp.CallToolResult, KafkaOut, error) {
		return t.kafkaRequest(ctx, "GET", "/kafka/regions", nil)
	})
	add(s, &mcp.Tool{Name: "kafka_list_clusters", Description: "List dedicated Kafka clusters in a project.", Annotations: readOnly("List Kafka clusters")}, func(ctx context.Context, _ *mcp.CallToolRequest, in KafkaProjectIn) (*mcp.CallToolResult, KafkaOut, error) {
		return t.kafkaRequest(ctx, "GET", kafkaPath(in.ProjectID, ""), nil)
	})
	add(s, &mcp.Tool{Name: "kafka_get_cluster", Description: "Read Kafka status, public and VPC bootstrap servers, SASL settings and CA. No password is returned.", Annotations: readOnly("Get Kafka cluster")}, func(ctx context.Context, _ *mcp.CallToolRequest, in KafkaClusterIn) (*mcp.CallToolResult, KafkaOut, error) {
		return t.kafkaRequest(ctx, "GET", kafkaPath(in.ProjectID, in.ClusterID), nil)
	})
	add(s, &mcp.Tool{Name: "kafka_create_cluster", Description: "Create paid dedicated managed Kafka. Dev uses one VM; HA uses three controllers and three brokers. The customer password is returned once; give it to the user and do not log it.", Annotations: additive("Create Kafka cluster", false)}, func(ctx context.Context, _ *mcp.CallToolRequest, in KafkaCreateIn) (*mcp.CallToolResult, KafkaOut, error) {
		allow := in.Allowlist
		if allow == nil {
			allow = []string{}
		}
		topology := in.Topology
		if topology == "" {
			topology = "dev"
		}
		body := map[string]any{"name": in.Name, "cluster_id": in.RegionID, "plan_id": in.PlanID, "topology": topology, "ip_allowlist": allow}
		if in.Version != "" {
			body["kafka_version"] = in.Version
		}
		if in.RetentionHours != 0 {
			body["retention_hours"] = in.RetentionHours
		}
		if in.ControllerPlanID != "" {
			body["controller_plan_id"] = in.ControllerPlanID
		}
		if in.VPCID != "" {
			body["vpc_id"] = in.VPCID
		}
		return t.kafkaRequest(ctx, "POST", kafkaPath(in.ProjectID, ""), body)
	})
	add(s, &mcp.Tool{Name: "kafka_update_allowlist", Description: "Replace the Kafka public IPv4 CIDR allowlist. Empty closes public access; VPC access remains enabled.", Annotations: destructive("Update Kafka allowlist")}, func(ctx context.Context, _ *mcp.CallToolRequest, in KafkaAllowlistIn) (*mcp.CallToolResult, KafkaOut, error) {
		return t.kafkaRequest(ctx, "PUT", kafkaPath(in.ProjectID, in.ClusterID)+"/allowlist", map[string]any{"ip_allowlist": in.Allowlist})
	})
	add(s, &mcp.Tool{Name: "kafka_delete_cluster", Description: "Delete dedicated Kafka cluster, all node VMs and all Kafka data.", Annotations: destructive("Delete Kafka cluster")}, func(ctx context.Context, _ *mcp.CallToolRequest, in KafkaClusterIn) (*mcp.CallToolResult, KafkaOut, error) {
		return t.kafkaRequest(ctx, "DELETE", kafkaPath(in.ProjectID, in.ClusterID), nil)
	})
}
