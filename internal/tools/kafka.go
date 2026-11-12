package tools

import (
	"context"
	"net/url"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
type KafkaOut = ObjectOut

func kafkaPath(project, id string) string {
	p := "/projects/" + url.PathEscape(project) + "/kafka-clusters"
	if id != "" {
		p += "/" + url.PathEscape(id)
	}
	return p
}
func (t *Tools) kafkaRequest(ctx context.Context, method, path string, body any) (*mcp.CallToolResult, KafkaOut, error) {
	return t.objectRequest(ctx, method, path, body)
}

type KafkaMetricsIn struct {
	ProjectID string `json:"project_id"`
	ClusterID string `json:"cluster_id"`
	NodeID    string `json:"node_id"`
	Metric    string `json:"metric" jsonschema:"cpu, memory, disk_io or network_io"`
	Range     string `json:"range,omitempty" jsonschema:"1h, 6h, 24h, 7d or 30d; default 1h"`
}

func (t *Tools) kafkaResetPassword(ctx context.Context, _ *mcp.CallToolRequest, in KafkaClusterIn) (*mcp.CallToolResult, KafkaOut, error) {
	return t.kafkaRequest(ctx, "POST", kafkaPath(in.ProjectID, in.ClusterID)+"/password", nil)
}
func (t *Tools) kafkaMetrics(ctx context.Context, _ *mcp.CallToolRequest, in KafkaMetricsIn) (*mcp.CallToolResult, KafkaOut, error) {
	if in.Range == "" {
		in.Range = "1h"
	}
	query := url.Values{"metric": {in.Metric}, "range": {in.Range}}
	return t.kafkaRequest(ctx, "GET", kafkaPath(in.ProjectID, in.ClusterID)+"/nodes/"+url.PathEscape(in.NodeID)+"/metrics?"+query.Encode(), nil)
}

type KafkaHistoryIn struct {
	ProjectID string `json:"project_id"`
	ClusterID string `json:"cluster_id"`
	Metric    string `json:"metric" jsonschema:"replication or consumer_lag"`
	Range     string `json:"range,omitempty" jsonschema:"1h, 6h, 24h, 7d or 30d; default 1h"`
	Group     string `json:"group,omitempty" jsonschema:"Exact consumer group for consumer_lag"`
}
type KafkaMonitoringConfigIn struct {
	ProjectID          string `json:"project_id"`
	ClusterID          string `json:"cluster_id"`
	LagThreshold       *int64 `json:"lag_threshold" jsonschema:"Positive total offset lag per group; null disables warnings"`
	LagDurationSeconds int    `json:"lag_duration_seconds" jsonschema:"60..3600 seconds, multiple of 60"`
}

func (t *Tools) kafkaHealth(ctx context.Context, _ *mcp.CallToolRequest, in KafkaClusterIn) (*mcp.CallToolResult, KafkaOut, error) {
	return t.kafkaRequest(ctx, "GET", kafkaPath(in.ProjectID, in.ClusterID)+"/monitoring", nil)
}
func (t *Tools) kafkaHistory(ctx context.Context, _ *mcp.CallToolRequest, in KafkaHistoryIn) (*mcp.CallToolResult, KafkaOut, error) {
	if in.Range == "" {
		in.Range = "1h"
	}
	query := url.Values{"metric": {in.Metric}, "range": {in.Range}}
	if in.Group != "" {
		query.Set("group", in.Group)
	}
	return t.kafkaRequest(ctx, "GET", kafkaPath(in.ProjectID, in.ClusterID)+"/monitoring/metrics?"+query.Encode(), nil)
}
func (t *Tools) kafkaMonitoringConfig(ctx context.Context, _ *mcp.CallToolRequest, in KafkaMonitoringConfigIn) (*mcp.CallToolResult, KafkaOut, error) {
	return t.kafkaRequest(ctx, "PUT", kafkaPath(in.ProjectID, in.ClusterID)+"/monitoring", map[string]any{"lag_threshold": in.LagThreshold, "lag_duration_seconds": in.LagDurationSeconds})
}
func (t *Tools) registerKafka(s *mcp.Server) {
	add(s, &mcp.Tool{Name: "kafka_health", Description: "Read Kafka replication health and committed-offset consumer lag. Unknown is not zero; check observation freshness. Requires kafka_cluster:read.", Annotations: readOnly("Kafka health")}, t.kafkaHealth)
	add(s, &mcp.Tool{Name: "kafka_history", Description: "Read tenant-scoped replication or group lag history. Gaps mean unknown. Requires kafka_cluster:read.", Annotations: readOnly("Kafka history")}, t.kafkaHistory)
	add(s, &mcp.Tool{Name: "kafka_monitoring_config", Description: "Set console lag warnings separately per group. Null threshold disables warnings. Applied asynchronously; sends no external notifications. Requires kafka_cluster:update.", Annotations: additive("Configure Kafka monitoring", true)}, t.kafkaMonitoringConfig)

	add(s, &mcp.Tool{Name: "kafka_reset_password", Description: "Replace the Kafka customer password. Requires kafka_cluster:update. Returns a new password once, applied=false until regional confirmation. Update all clients; never log the password.", Annotations: destructive("Reset Kafka password")}, t.kafkaResetPassword)
	add(s, &mcp.Tool{Name: "kafka_node_metrics", Description: "Read VM CPU, memory, disk I/O or network metrics for an owned Kafka node. Requires kafka_cluster:read. This is not Kafka ISR or consumer lag telemetry.", Annotations: readOnly("Kafka node metrics")}, t.kafkaMetrics)

	add(s, &mcp.Tool{Name: "kafka_resume_cluster", Description: "Resume Kafka after payment. Billing charges one hour for stopped subscriptions before starting nodes; paid subscriptions are not charged again. Requires kafka_cluster:update.", Annotations: additive("Resume paid Kafka", false)}, t.kafkaResume)
	add(s, &mcp.Tool{Name: "kafka_canary_node_power", Description: "Closed canary start/stop verification for an owned Kafka node. Requires configured canary account and kafka_cluster:update; VM billing continues.", Annotations: destructive("Kafka canary node power")}, func(ctx context.Context, _ *mcp.CallToolRequest, in KafkaCanaryPowerIn) (*mcp.CallToolResult, KafkaOut, error) {
		return t.kafkaRequest(ctx, "POST", kafkaPath(in.ProjectID, in.ClusterID)+"/nodes/"+url.PathEscape(in.NodeID)+"/canary-power", map[string]string{"action": in.Action})
	})

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

func (t *Tools) kafkaResume(ctx context.Context, _ *mcp.CallToolRequest, in KafkaClusterIn) (*mcp.CallToolResult, KafkaOut, error) {
	return t.kafkaRequest(ctx, "POST", kafkaPath(in.ProjectID, in.ClusterID)+"/resume", nil)
}

type KafkaCanaryPowerIn struct {
	ProjectID string `json:"project_id"`
	ClusterID string `json:"cluster_id"`
	NodeID    string `json:"node_id"`
	Action    string `json:"action" jsonschema:"start or stop; only configured canary accounts; billing continues"`
}
