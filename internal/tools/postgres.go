package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tatnet-ru/tatnet-go/tatnet"
)

type PgReadReplicasIn struct {
	ProjectID string `json:"project_id" jsonschema:"project UUID"`
	ClusterID string `json:"cluster_id" jsonschema:"managed PostgreSQL cluster UUID"`
	Count     int    `json:"count" jsonschema:"number of dedicated paid asynchronous read replicas, from 0 to 5; zero removes read replicas"`
}

type PgClusterOut struct {
	Cluster map[string]any `json:"cluster"`
}

type PgClusterIn struct {
	ProjectID string `json:"project_id" jsonschema:"project UUID"`
	ClusterID string `json:"cluster_id" jsonschema:"managed PostgreSQL cluster UUID"`
}

func (t *Tools) registerPostgres(s *mcp.Server) {
	add(s, &mcp.Tool{Name: "get_postgres_cluster", Description: "Inspect a managed PostgreSQL cluster, its read endpoint, replica nodes, lag and pending changes. Use this to verify readiness after setting read replicas.", Annotations: readOnly("PostgreSQL cluster")},
		func(ctx context.Context, _ *mcp.CallToolRequest, in PgClusterIn) (*mcp.CallToolResult, PgClusterOut, error) {
			var out PgClusterOut
			project, err := uuid.Parse(in.ProjectID)
			if err != nil {
				return nil, out, fmt.Errorf("invalid project UUID")
			}
			cluster, err := uuid.Parse(in.ClusterID)
			if err != nil {
				return nil, out, fmt.Errorf("invalid cluster UUID")
			}
			c, err := t.client(ctx)
			if err != nil {
				return nil, out, err
			}
			r, err := c.PostgresGetClusterWithResponse(ctx, project.String(), cluster.String())
			if err := check("get PostgreSQL cluster", r, err); err != nil {
				return nil, out, err
			}
			err = json.Unmarshal(r.Body, &out.Cluster)
			return nil, out, err
		})

	add(s, &mcp.Tool{Name: "set_postgres_read_replicas",
		Description: "Set dedicated asynchronous PostgreSQL read replicas, billed at the cluster node plan. Independent of HA. Replicas never automatically promote. Read host/port and pending state are returned; recorded intent does not mean replication is ready.",
		Annotations: &mcp.ToolAnnotations{Title: "Set PostgreSQL read replicas", ReadOnlyHint: false, DestructiveHint: ptr(true), IdempotentHint: true, OpenWorldHint: ptr(false)},
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in PgReadReplicasIn) (*mcp.CallToolResult, PgClusterOut, error) {
		out, err := t.setPgReadReplicas(ctx, in)
		return nil, out, err
	})
}

func (t *Tools) setPgReadReplicas(ctx context.Context, in PgReadReplicasIn) (PgClusterOut, error) {
	var out PgClusterOut
	project, err := uuid.Parse(in.ProjectID)
	if err != nil {
		return out, fmt.Errorf("invalid project UUID")
	}
	cluster, err := uuid.Parse(in.ClusterID)
	if err != nil {
		return out, fmt.Errorf("invalid cluster UUID")
	}
	if in.Count < 0 || in.Count > 5 {
		return out, fmt.Errorf("count must be between 0 and 5")
	}
	c, err := t.client(ctx)
	if err != nil {
		return out, err
	}
	resp, err := c.PostgresChangeReadReplicasWithResponse(ctx, project.String(), cluster.String(), tatnet.ChangePgReadReplicasRequest{ReadReplicaCount: in.Count})
	if err := check("set PostgreSQL read replicas", resp, err); err != nil {
		return out, err
	}
	err = json.Unmarshal(resp.Body, &out.Cluster)
	return out, err
}
