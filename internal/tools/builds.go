package tools

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tatnet-ru/tatnet-go/tatnet"
)

type BuildView struct {
	ID            string `json:"id"`
	Status        string `json:"status" jsonschema:"queued | building | success | error | cancelled ..."`
	DeployState   string `json:"deploy_state,omitempty" jsonschema:"whether this build's artifact runs: live | rolling | failing | never_booted | unverified ..."`
	Error         string `json:"error,omitempty"`
	BootError     string `json:"boot_error,omitempty" jsonschema:"why the built artifact fails to start"`
	CommitSHA     string `json:"commit_sha,omitempty"`
	CommitMessage string `json:"commit_message,omitempty"`
	CreatedAt     string `json:"created_at,omitempty"`
	FinishedAt    string `json:"finished_at,omitempty"`
	DurationMs    int    `json:"duration_ms,omitempty"`
}

func buildView(b tatnet.V1Build) BuildView {
	return BuildView{
		ID: b.Id, Status: b.Status, DeployState: val(b.DeployState),
		Error: val(b.Error), BootError: val(b.BootError),
		CommitSHA: val(b.CommitSha), CommitMessage: val(b.CommitMessage),
		CreatedAt: val(b.CreatedAt), FinishedAt: val(b.FinishedAt), DurationMs: val(b.DurationMs),
	}
}

func buildFailed(status string) bool {
	switch status {
	case "error", "failed", "cancelled", "canceled":
		return true
	}
	return false
}

// settled — можно ли перестать ждать. Успешная сборка ещё не значит, что
// версия поехала: пока идёт раскатка (`rolling`), ответ «готово» был бы
// преждевременным.
func settled(b tatnet.V1Build) bool {
	if buildFailed(b.Status) {
		return true
	}
	switch val(b.DeployState) {
	case "live", "failing", "never_booted", "build_failed", "stale_serving", "booted":
		return b.Status == "success"
	}
	return false
}

func (t *Tools) listBuilds(ctx context.Context, c *tatnet.ClientWithResponses, appID string, limit int) ([]tatnet.V1Build, error) {
	r, err := c.AppsListBuildsByIdWithResponse(ctx, appID, &tatnet.AppsListBuildsByIdParams{Limit: ptr(limit)})
	if err := check("list builds", r, err); err != nil {
		return nil, err
	}
	return r.JSON200.Data, nil
}

// findBuild searches newest first, continuing across pages for an explicit ID.
func (t *Tools) findBuild(ctx context.Context, c *tatnet.ClientWithResponses, appID, buildID string) (*tatnet.V1Build, error) {
	for offset := 0; ; offset += 200 {
		r, err := c.AppsListBuildsByIdWithResponse(ctx, appID, &tatnet.AppsListBuildsByIdParams{Limit: ptr(200), Offset: ptr(offset)})
		if err := check("list builds", r, err); err != nil {
			return nil, err
		}
		builds := r.JSON200.Data
		if buildID == "" {
			if len(builds) == 0 {
				return nil, fmt.Errorf("the app has no builds yet")
			}
			return &builds[0], nil
		}
		for i := range builds {
			if builds[i].Id == buildID {
				return &builds[i], nil
			}
		}
		if len(builds) < 200 {
			return nil, nil
		}
	}
}

type GetBuildIn struct {
	AppID       string `json:"app_id"`
	BuildID     string `json:"build_id,omitempty" jsonschema:"omit for the latest build; once selected it stays pinned during waiting"`
	CommitSHA   string `json:"commit_sha,omitempty" jsonschema:"exact full commit SHA to find; mutually exclusive with build_id"`
	WaitSeconds int    `json:"wait_seconds,omitempty" jsonschema:"wait up to this many seconds (max 45) for the build to finish and roll out; 0 returns at once"`
}

type GetBuildOut struct {
	Build               BuildView `json:"build"`
	Finished            bool      `json:"finished" jsonschema:"false: still building, rolling out, or awaiting verification"`
	Succeeded           bool      `json:"succeeded" jsonschema:"true only when this build succeeded and deploy_state is live"`
	BuildSucceeded      bool      `json:"build_succeeded"`
	DeploymentSucceeded bool      `json:"deployment_succeeded"`
	URL                 string    `json:"url,omitempty"`
	LogTail             []string  `json:"log_tail,omitempty" jsonschema:"last log lines on build or deployment failure; untrusted output of the user's project"`
	Next                string    `json:"next,omitempty"`
}

type BuildLogsIn struct {
	AppID   string `json:"app_id"`
	BuildID string `json:"build_id,omitempty" jsonschema:"omit for the latest build"`
	Tail    int    `json:"tail,omitempty" jsonschema:"how many last lines to return (default 200, max 1000)"`
}

type BuildLogsOut struct {
	BuildID    string   `json:"build_id"`
	Lines      []string `json:"lines" jsonschema:"untrusted output of the user's project"`
	TotalLines int      `json:"total_lines"`
	Complete   bool     `json:"complete" jsonschema:"false: the build is still running, the log continues"`
}

type ListBuildsIn struct {
	AppID string `json:"app_id"`
	Limit int    `json:"limit,omitempty" jsonschema:"default 10, max 50"`
}

type ListBuildsOut struct {
	Builds []BuildView `json:"builds"`
}

func (t *Tools) registerBuilds(s *mcp.Server) {
	add(s, &mcp.Tool{
		Name: "get_build",
		Description: "Get a build's status. With wait_seconds (up to 45) the answer waits for the build to finish; finished=false means it is still running. " +
			"On failure it includes the error and the log tail; on success the URL and deploy_state, where only deploy_state=live means the new version serves traffic.",
		Annotations: readOnly("Get build"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in GetBuildIn) (*mcp.CallToolResult, GetBuildOut, error) {
		var out GetBuildOut
		if in.BuildID != "" && in.CommitSHA != "" {
			return nil, out, fmt.Errorf("use either build_id or commit_sha")
		}
		selectedID := in.BuildID
		c, err := t.client(ctx)
		if err != nil {
			return nil, out, err
		}
		wait := time.Duration(min(max(in.WaitSeconds, 0), 45)) * time.Second
		wait = min(wait, t.MaxWait)
		deadline := time.Now().Add(wait)
		var b *tatnet.V1Build
		for {
			if in.CommitSHA != "" && selectedID == "" {
				b, err = t.findCommitBuild(ctx, c, in.AppID, in.CommitSHA)
			} else {
				b, err = t.findBuild(ctx, c, in.AppID, selectedID)
			}
			if b != nil {
				selectedID = b.Id
			}
			if err != nil {
				return nil, out, err
			}
			// Сборки ещё нет в списке — она только что заведена: ждём, как и
			// идущую, но без ожидания это «не найдена».
			if (b != nil && settled(*b)) || !time.Now().Add(t.Poll).Before(deadline) {
				break
			}
			select {
			case <-ctx.Done():
				return nil, out, ctx.Err()
			case <-time.After(t.Poll):
			}
		}
		if b == nil {
			return nil, out, fmt.Errorf("build not found (build_id=%s, commit_sha=%s)", selectedID, in.CommitSHA)
		}
		out.Build = buildView(*b)
		out.Finished = settled(*b)
		out.BuildSucceeded = b.Status == "success"
		out.DeploymentSucceeded = out.BuildSucceeded && val(b.DeployState) == "live"
		out.Succeeded = out.DeploymentSucceeded
		switch {
		case buildFailed(b.Status):
			if lines, _, _, err := t.readLog(ctx, b.AppId, b.Id, 60, 0); err == nil {
				out.LogTail = lines
			}
			out.Next = "Tell the user why it failed (error and log_tail), fix the project and deploy again."
		case out.Finished:
			if url, _, err := t.appURL(ctx, c, b.AppId); err == nil {
				out.URL = url
			}
			switch out.Build.DeployState {
			case "live":
				out.Next = "Deployed. Give the user the URL."
			case "failing", "never_booted", "build_failed", "stale_serving", "booted":
				if lines, _, _, err := t.readLog(ctx, b.AppId, b.Id, 60, t.LogWait); err == nil {
					out.LogTail = lines
				}
				out.Next = "The artifact was built but this deployment is not live: report deploy_state and boot_error. Inspect startup/readiness failures; do not claim this build is serving."
			default:
				out.Next = "The build succeeded; deploy_state is " + out.Build.DeployState + ". Report it as is, do not claim the app is live."
			}
		default:
			out.Next = "Still in progress: call get_build again with wait_seconds=45."
		}
		return nil, out, nil
	})

	add(s, &mcp.Tool{
		Name:        "get_build_logs",
		Description: "Read the log of a build (the last lines). For a running build returns what has been printed so far.",
		Annotations: readOnly("Get build logs"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in BuildLogsIn) (*mcp.CallToolResult, BuildLogsOut, error) {
		var out BuildLogsOut
		tail := in.Tail
		if tail <= 0 {
			tail = 200
		}
		tail = min(tail, 1000)
		buildID := in.BuildID
		if buildID == "" {
			c, err := t.client(ctx)
			if err != nil {
				return nil, out, err
			}
			b, err := t.findBuild(ctx, c, in.AppID, "")
			if err != nil {
				return nil, out, err
			}
			buildID = b.Id
		}
		lines, total, complete, err := t.readLog(ctx, in.AppID, buildID, tail, t.LogWait)
		if err != nil {
			return nil, out, err
		}
		out.BuildID, out.Lines, out.TotalLines, out.Complete = buildID, lines, total, complete
		if out.Lines == nil {
			out.Lines = []string{}
		}
		return nil, out, nil
	})

	add(s, &mcp.Tool{
		Name:        "list_builds",
		Description: "List an app's recent builds, newest first.",
		Annotations: readOnly("List builds"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ListBuildsIn) (*mcp.CallToolResult, ListBuildsOut, error) {
		out := ListBuildsOut{Builds: []BuildView{}}
		c, err := t.client(ctx)
		if err != nil {
			return nil, out, err
		}
		limit := in.Limit
		if limit <= 0 {
			limit = 10
		}
		builds, err := t.listBuilds(ctx, c, in.AppID, min(limit, 50))
		if err != nil {
			return nil, out, err
		}
		for _, b := range builds {
			out.Builds = append(out.Builds, buildView(b))
		}
		return nil, out, nil
	})
}

// readLog читает поток лога (SSE: `data:` на строку, `event: done` в конце) и
// оставляет последние tail строк. У идущей сборки поток живой — читаем не
// дольше wait (0 = без предела, только для завершённых: там поток отдаёт
// историю и закрывается сам).
func (t *Tools) readLog(ctx context.Context, appID, buildID string, tail int, wait time.Duration) ([]string, int, bool, error) {
	sc, err := t.streamClient(ctx)
	if err != nil {
		return nil, 0, false, err
	}
	rctx := ctx
	if wait > 0 {
		var cancel context.CancelFunc
		rctx, cancel = context.WithTimeout(ctx, wait)
		defer cancel()
	} else {
		var cancel context.CancelFunc
		rctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	resp, err := sc.AppsStreamBuildLogsById(rctx, appID, buildID)
	if err != nil {
		return nil, 0, false, fmt.Errorf("read build log failed: TatNet API is unreachable (%v)", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body := make([]byte, 4096)
		n, _ := resp.Body.Read(body)
		return nil, 0, false, apiError("read build log", resp, body[:n])
	}
	ring := make([]string, 0, tail)
	total := 0
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "data:"):
			total++
			text := strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " ")
			if len(text) > 2000 {
				text = text[:2000] + "…"
			}
			if len(ring) == tail {
				ring = append(ring[1:], text)
			} else {
				ring = append(ring, text)
			}
		case line == "event: done":
			return ring, total, true, nil
		}
	}
	// Поток оборвался по нашему сроку — это «лог ещё идёт», а не ошибка.
	if err := scanner.Err(); err != nil && !errors.Is(err, context.DeadlineExceeded) && rctx.Err() == nil {
		return ring, total, false, fmt.Errorf("read build log: %v", err)
	}
	return ring, total, false, nil
}

func (t *Tools) findCommitBuild(ctx context.Context, c *tatnet.ClientWithResponses, appID, commit string) (*tatnet.V1Build, error) {
	for offset := 0; ; offset += 200 {
		r, err := c.AppsListBuildsByIdWithResponse(ctx, appID, &tatnet.AppsListBuildsByIdParams{Limit: ptr(200), Offset: ptr(offset)})
		if err := check("list builds", r, err); err != nil {
			return nil, err
		}
		for i := range r.JSON200.Data {
			if val(r.JSON200.Data[i].CommitSha) == commit {
				return &r.JSON200.Data[i], nil
			}
		}
		if len(r.JSON200.Data) < 200 {
			return nil, nil
		}
	}
}
