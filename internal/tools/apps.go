package tools

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tatnet-ru/tatnet-go/tatnet"

	"github.com/tatnet-ru/tatnet-mcp/internal/pack"
)

// ---------- представления ----------

type ProjectView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type AppView struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ProjectID      string `json:"project_id"`
	URL            string `json:"url,omitempty" jsonschema:"public address of the app (only in get_app and deploy results)"`
	SourceType     string `json:"source_type" jsonschema:"git | docker_image | upload (files deployed with deploy_files)"`
	AppType        string `json:"app_type,omitempty" jsonschema:"frontend | backend"`
	BuildStatus    string `json:"build_status" jsonschema:"status of the last build"`
	DeployState    string `json:"deploy_state,omitempty" jsonschema:"whether the built artifact actually runs: live | rolling | failing | never_booted | stale_serving | unverified | build_failed"`
	CurrentBuildID string `json:"current_build_id,omitempty"`
	Repo           string `json:"repo,omitempty"`
	Branch         string `json:"branch,omitempty"`
	DockerImage    string `json:"docker_image,omitempty"`
	ReplicaCount   int    `json:"replica_count" jsonschema:"0 = scale to zero (starts on first request)"`
	SourceError    string `json:"source_error,omitempty" jsonschema:"why the app's source cannot be fetched, if it cannot"`
}

func appView(a tatnet.V1App) AppView {
	return AppView{
		ID: a.Id, Name: a.Name, ProjectID: a.ProjectId,
		SourceType: orDefault(val(a.SourceType), "git"), AppType: val(a.AppType),
		BuildStatus: a.Status, DeployState: val(a.DeployState), CurrentBuildID: val(a.CurrentBuildId),
		Repo: val(a.RepoFullName), Branch: val(a.Branch), DockerImage: val(a.DockerImage),
		ReplicaCount: val(a.ReplicaCount), SourceError: val(a.SourceError),
	}
}

// ---------- whoami / список ----------

type WhoamiIn struct{}

type WhoamiOut struct {
	AccountID string        `json:"account_id"`
	Projects  []ProjectView `json:"projects"`
	Scoped    bool          `json:"key_is_scoped" jsonschema:"true if the API key is limited to some projects or actions: lists then show only what the key may see"`
	Note      string        `json:"note,omitempty"`
}

type ListAppsIn struct {
	ProjectID string `json:"project_id,omitempty" jsonschema:"only apps of this project; omit for all projects"`
}

type ListAppsOut struct {
	Apps      []AppView `json:"apps"`
	Truncated bool      `json:"truncated,omitempty"`
	Note      string    `json:"note,omitempty"`
}

type GetAppIn struct {
	AppID string `json:"app_id"`
}

type GetAppOut struct {
	App         AppView      `json:"app"`
	Domains     []DomainView `json:"domains"`
	LatestBuild *BuildView   `json:"latest_build,omitempty"`
}

const listCap = 1000

func (t *Tools) registerAccount(s *mcp.Server) {
	add(s, &mcp.Tool{
		Name:        "whoami",
		Description: "Show the TatNet account this connection acts as and the projects it can use. Call first to get a project_id.",
		Annotations: readOnly("Who am I"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, _ WhoamiIn) (*mcp.CallToolResult, WhoamiOut, error) {
		var out WhoamiOut
		c, err := t.client(ctx)
		if err != nil {
			return nil, out, err
		}
		w, err := c.AccountWhoamiWithResponse(ctx)
		if err := check("whoami", w, err); err != nil {
			return nil, out, err
		}
		out.AccountID = w.JSON200.AccountId
		out.Scoped = policyScoped(w.JSON200.Policy)
		projects, err := t.listProjects(ctx, c)
		if err != nil {
			return nil, out, err
		}
		out.Projects = projects
		if len(projects) == 0 {
			out.Note = emptyNote(out.Scoped, "projects")
		}
		return nil, out, nil
	})
}

func policyScoped(policy []tatnet.V1PolicyStatement) bool {
	for _, st := range policy {
		if strings.EqualFold(val(st.Effect), "deny") {
			return true
		}
		for _, r := range st.Resources {
			if r != "*" {
				return true
			}
		}
	}
	return false
}

// emptyNote: пустой список у /v1 бывает и «записей нет», и «ключу не видно»
// — api отвечает 200 и пусто в обоих случаях. Модель не должна уверенно
// сообщать «у вас ничего нет», когда это ограничение ключа.
func emptyNote(scoped bool, what string) string {
	if scoped {
		return fmt.Sprintf("No %s visible. This API key is limited to some projects, so this may be the key's restriction rather than an absence; the user can widen the key in the TatNet dashboard.", what)
	}
	return fmt.Sprintf("No %s in this account.", what)
}

func (t *Tools) scoped(ctx context.Context, c *tatnet.ClientWithResponses) bool {
	w, err := c.AccountWhoamiWithResponse(ctx)
	if check("whoami", w, err) != nil {
		// Не смогли узнать — говорим осторожную версию.
		return true
	}
	return policyScoped(w.JSON200.Policy)
}

func (t *Tools) listProjects(ctx context.Context, c *tatnet.ClientWithResponses) ([]ProjectView, error) {
	var out []ProjectView
	// `count` у /v1 — размер страницы, а не всего: листаем до короткой страницы.
	for offset := 0; offset < listCap; offset += 200 {
		r, err := c.ProjectsListProjectsWithResponse(ctx, &tatnet.ProjectsListProjectsParams{Limit: ptr(200), Offset: ptr(offset)})
		if err := check("list projects", r, err); err != nil {
			return nil, err
		}
		for _, p := range r.JSON200.Data {
			out = append(out, ProjectView{ID: p.Id, Name: p.Name})
		}
		if len(r.JSON200.Data) < 200 {
			break
		}
	}
	return out, nil
}

func (t *Tools) listApps(ctx context.Context, c *tatnet.ClientWithResponses, projectID string) ([]tatnet.V1App, bool, error) {
	var out []tatnet.V1App
	params := &tatnet.AppsListAppsByAccountParams{Limit: ptr(200)}
	if projectID != "" {
		params.ProjectId = ptr(projectID)
	}
	for offset := 0; offset < listCap; offset += 200 {
		params.Offset = ptr(offset)
		r, err := c.AppsListAppsByAccountWithResponse(ctx, params)
		if err := check("list apps", r, err); err != nil {
			return nil, false, err
		}
		out = append(out, r.JSON200.Data...)
		if len(r.JSON200.Data) < 200 {
			return out, false, nil
		}
	}
	return out, true, nil
}

func (t *Tools) getApp(ctx context.Context, c *tatnet.ClientWithResponses, appID string) (tatnet.V1App, error) {
	r, err := c.AppsGetAppByIdWithResponse(ctx, appID)
	if err := check("get app", r, err); err != nil {
		return tatnet.V1App{}, err
	}
	return *r.JSON200, nil
}

// appURL — адрес по умолчанию, иначе первый домен. Тот же выбор, что у
// `tatnet deploy`.
func (t *Tools) appURL(ctx context.Context, c *tatnet.ClientWithResponses, appID string) (string, []DomainView, error) {
	domains, err := t.listDomains(ctx, c, appID)
	if err != nil {
		return "", nil, err
	}
	best := ""
	for _, d := range domains {
		if d.IsDefault {
			best = d.Domain
			break
		}
		if best == "" {
			best = d.Domain
		}
	}
	if best == "" {
		return "", domains, nil
	}
	return "https://" + best, domains, nil
}

// ---------- create / deploy ----------

type CreateAppIn struct {
	ProjectID       string `json:"project_id"`
	Name            string `json:"name" jsonschema:"app name: lowercase letters, digits and dashes; becomes part of the default URL"`
	SourceType      string `json:"source_type" jsonschema:"git or docker_image. To publish files from the conversation use deploy_files instead"`
	Repo            string `json:"repo,omitempty" jsonschema:"git: owner/name of a repository connected to TatNet"`
	Branch          string `json:"branch,omitempty" jsonschema:"git: branch to deploy (default main)"`
	GitProvider     string `json:"git_provider,omitempty" jsonschema:"git: github (default) | gitlab | gitea | gitverse | gitflic"`
	GitConnectionID string `json:"git_connection_id,omitempty" jsonschema:"git: id of the account's git connection, if the provider needs it"`
	DockerImage     string `json:"docker_image,omitempty" jsonschema:"docker_image: image reference, e.g. ghcr.io/org/app:1.2.3"`
	AppType         string `json:"app_type,omitempty" jsonschema:"frontend (static or SSR sites, default) or backend (long-running server listening on $PORT)"`
	Framework       string `json:"framework,omitempty" jsonschema:"framework id if auto-detection should be overridden (e.g. nextjs, vite, static)"`
	BuildCommand    string `json:"build_command,omitempty"`
	InstallCommand  string `json:"install_command,omitempty"`
	OutputDirectory string `json:"output_directory,omitempty"`
	RootDirectory   string `json:"root_directory,omitempty"`
	StartCommand    string `json:"start_command,omitempty" jsonschema:"backend: command that starts the server"`
	ReplicaCount    *int   `json:"replica_count,omitempty" jsonschema:"backend: always-on replicas (0 = scale to zero, the default)"`
}

type CreateAppOut struct {
	App            AppView `json:"app"`
	AlreadyExisted bool    `json:"already_existed" jsonschema:"an app with this name already existed in the project and was returned instead of creating a duplicate"`
	Next           string  `json:"next"`
}

type DeployFilesIn struct {
	ProjectID       string      `json:"project_id,omitempty" jsonschema:"project for a new app (required unless app_id is given)"`
	AppID           string      `json:"app_id,omitempty" jsonschema:"redeploy into this existing app (it must have been created by deploy_files)"`
	Name            string      `json:"name,omitempty" jsonschema:"app name for a new app; an existing file-deployed app with this name in the project is reused"`
	Files           []pack.File `json:"files" jsonschema:"the complete project: every file the build needs (package.json, sources, assets). Environment files (.env) are skipped: use set_env"`
	Framework       string      `json:"framework,omitempty" jsonschema:"new app only: override framework auto-detection"`
	BuildCommand    string      `json:"build_command,omitempty" jsonschema:"new app only"`
	InstallCommand  string      `json:"install_command,omitempty" jsonschema:"new app only"`
	OutputDirectory string      `json:"output_directory,omitempty" jsonschema:"new app only"`
}

type DeployOut struct {
	AppID        string   `json:"app_id"`
	AppName      string   `json:"app_name,omitempty"`
	AppCreated   bool     `json:"app_created,omitempty"`
	BuildID      string   `json:"build_id"`
	Status       string   `json:"status"`
	URL          string   `json:"url,omitempty" jsonschema:"where the app will be served once the build is live"`
	FilesPacked  int      `json:"files_packed,omitempty"`
	SkippedFiles []string `json:"skipped_files,omitempty" jsonschema:"environment files left out on purpose; set their variables with set_env"`
	Next         string   `json:"next"`
}

type DeployAppIn struct {
	AppID     string `json:"app_id"`
	CommitSHA string `json:"commit_sha,omitempty" jsonschema:"git: build this commit instead of the branch head"`
}

const nextWait = "Call get_build with this build_id and wait_seconds=45, repeating until finished is true; then report the URL and deploy_state."

func (t *Tools) registerApps(s *mcp.Server) {
	add(s, &mcp.Tool{
		Name:        "list_apps",
		Description: "List apps with their last build status and deploy_state.",
		Annotations: readOnly("List apps"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ListAppsIn) (*mcp.CallToolResult, ListAppsOut, error) {
		out := ListAppsOut{Apps: []AppView{}}
		c, err := t.client(ctx)
		if err != nil {
			return nil, out, err
		}
		apps, truncated, err := t.listApps(ctx, c, in.ProjectID)
		if err != nil {
			return nil, out, err
		}
		for _, a := range apps {
			out.Apps = append(out.Apps, appView(a))
		}
		out.Truncated = truncated
		if len(apps) == 0 {
			out.Note = emptyNote(t.scoped(ctx, c), "apps")
		}
		return nil, out, nil
	})

	add(s, &mcp.Tool{
		Name:        "get_app",
		Description: "Get an app: its URL, domains, settings and the latest build.",
		Annotations: readOnly("Get app"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in GetAppIn) (*mcp.CallToolResult, GetAppOut, error) {
		var out GetAppOut
		c, err := t.client(ctx)
		if err != nil {
			return nil, out, err
		}
		a, err := t.getApp(ctx, c, in.AppID)
		if err != nil {
			return nil, out, err
		}
		out.App = appView(a)
		url, domains, err := t.appURL(ctx, c, a.Id)
		if err != nil {
			return nil, out, err
		}
		out.App.URL, out.Domains = url, domains
		builds, err := t.listBuilds(ctx, c, a.Id, 1)
		if err != nil {
			return nil, out, err
		}
		if len(builds) > 0 {
			b := buildView(builds[0])
			out.LatestBuild = &b
		}
		return nil, out, nil
	})

	add(s, &mcp.Tool{
		Name: "create_app",
		Description: "Create an app that deploys from a git repository or a Docker image. " +
			"Does not start a build: call deploy_app next. Idempotent by name: an existing app with the same name in the project is returned instead of a duplicate. " +
			"To publish code written in the conversation, use deploy_files instead.",
		Annotations: additive("Create app", true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in CreateAppIn) (*mcp.CallToolResult, CreateAppOut, error) {
		var out CreateAppOut
		switch in.SourceType {
		case "git", "docker_image":
		case "upload":
			return nil, out, fmt.Errorf("use deploy_files to publish files; create_app is for git and docker_image apps")
		default:
			return nil, out, fmt.Errorf("source_type must be git or docker_image")
		}
		if in.ProjectID == "" || in.Name == "" {
			return nil, out, fmt.Errorf("project_id and name are required (whoami lists projects)")
		}
		c, err := t.client(ctx)
		if err != nil {
			return nil, out, err
		}
		existing, err := t.findByName(ctx, c, in.ProjectID, in.Name)
		if err != nil {
			return nil, out, err
		}
		if existing != nil {
			out.App, out.AlreadyExisted = appView(*existing), true
			out.Next = "An app with this name already exists; call deploy_app with its id to build it."
			return nil, out, nil
		}
		body := tatnet.V1AppCreate{
			Name: in.Name, SourceType: ptr(in.SourceType),
			AppType: opt(in.AppType), Framework: opt(in.Framework),
			BuildCommand: opt(in.BuildCommand), InstallCommand: opt(in.InstallCommand),
			OutputDirectory: opt(in.OutputDirectory), RootDirectory: opt(in.RootDirectory),
			StartCommand: opt(in.StartCommand), ReplicaCount: in.ReplicaCount,
		}
		if in.SourceType == "git" {
			if in.Repo == "" {
				return nil, out, fmt.Errorf("repo (owner/name) is required for a git app")
			}
			body.RepoFullName, body.Branch = ptr(in.Repo), opt(in.Branch)
			body.GitProvider, body.GitConnectionId = opt(in.GitProvider), opt(in.GitConnectionID)
		} else {
			if in.DockerImage == "" {
				return nil, out, fmt.Errorf("docker_image is required for a docker_image app")
			}
			body.DockerImage = ptr(in.DockerImage)
			if in.AppType == "" {
				body.AppType = ptr("backend")
			}
		}
		r, err := c.AppsCreateAppWithResponse(ctx, in.ProjectID, body)
		if err := check("create app", r, err); err != nil {
			return nil, out, err
		}
		out.App = appView(*r.JSON201)
		out.Next = "Call deploy_app with app.id to start the first build."
		return nil, out, nil
	})

	add(s, &mcp.Tool{
		Name: "deploy_files",
		Description: "Publish a WEBSITE from files (static site or SSR framework such as Next.js, Vite, Astro): pack them, upload and start a build. " +
			"Creates the app on first use (or reuses the file-deployed app with the same name), so calling it again with the same project_id and name redeploys. " +
			"Send the COMPLETE project every time: files not sent are not in the new version. Up to 20 MiB, 2000 files. " +
			"Long-running backend services (APIs, bots, workers) cannot be deployed from files yet: push them to a git repository or a Docker image and use create_app.",
		Annotations: additive("Deploy files", false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in DeployFilesIn) (*mcp.CallToolResult, DeployOut, error) {
		var out DeployOut
		// Пакуем ДО создания аппа: битые файлы не должны оставлять пустой апп.
		packed, err := pack.Build(in.Files)
		if err != nil {
			return nil, out, err
		}
		c, err := t.client(ctx)
		if err != nil {
			return nil, out, err
		}
		app, created, err := t.resolveUploadApp(ctx, c, in)
		if err != nil {
			return nil, out, err
		}
		out.AppID, out.AppName, out.AppCreated = app.Id, app.Name, created
		r, err := c.AppsCreateDeploymentByIdWithBodyWithResponse(ctx, app.Id, "application/gzip", bytes.NewReader(packed.Archive))
		if err := check("deploy files", r, err); err != nil {
			return nil, out, err
		}
		out.BuildID, out.Status = r.JSON202.Id, r.JSON202.Status
		out.FilesPacked, out.SkippedFiles = packed.Files, packed.Skipped
		if url, _, err := t.appURL(ctx, c, app.Id); err == nil {
			out.URL = url
		}
		out.Next = nextWait
		return nil, out, nil
	})

	add(s, &mcp.Tool{
		Name:        "deploy_app",
		Description: "Start a new build and deploy of a git or Docker-image app (the latest commit of its branch, or commit_sha). For file-deployed apps use deploy_files.",
		Annotations: additive("Deploy app", false),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in DeployAppIn) (*mcp.CallToolResult, DeployOut, error) {
		var out DeployOut
		c, err := t.client(ctx)
		if err != nil {
			return nil, out, err
		}
		a, err := t.getApp(ctx, c, in.AppID)
		if err != nil {
			return nil, out, err
		}
		if val(a.SourceType) == "upload" {
			return nil, out, fmt.Errorf("app %q deploys from files: call deploy_files with app_id and the complete project", a.Name)
		}
		r, err := c.AppsDeployAppByIdWithResponse(ctx, a.Id, tatnet.V1DeployRequest{CommitSha: opt(in.CommitSHA)})
		if err := check("deploy app", r, err); err != nil {
			return nil, out, err
		}
		out.AppID, out.AppName, out.BuildID, out.Status = a.Id, a.Name, r.JSON200.Id, r.JSON200.Status
		if url, _, err := t.appURL(ctx, c, a.Id); err == nil {
			out.URL = url
		}
		out.Next = nextWait
		return nil, out, nil
	})
}

func opt(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return ptr(s)
}

func (t *Tools) findByName(ctx context.Context, c *tatnet.ClientWithResponses, projectID, name string) (*tatnet.V1App, error) {
	apps, _, err := t.listApps(ctx, c, projectID)
	if err != nil {
		return nil, err
	}
	for i := range apps {
		if apps[i].Name == name {
			return &apps[i], nil
		}
	}
	return nil, nil
}

// resolveUploadApp: заданный апп, иначе апп с тем же именем, иначе новый.
// Повторный вызов с тем же именем обязан попасть в тот же апп — модели
// повторяют вызовы, и каждый повтор иначе плодил бы новый адрес.
func (t *Tools) resolveUploadApp(ctx context.Context, c *tatnet.ClientWithResponses, in DeployFilesIn) (tatnet.V1App, bool, error) {
	if in.AppID != "" {
		a, err := t.getApp(ctx, c, in.AppID)
		if err != nil {
			return a, false, err
		}
		if st := orDefault(val(a.SourceType), "git"); st != "upload" {
			// Залить папку в git-апп молча нельзя: следующий пуш перетёр бы её.
			return a, false, fmt.Errorf("app %q deploys from %s, not from files; use deploy_app, or deploy_files with a new name", a.Name, st)
		}
		if err := backendFromFiles(a); err != nil {
			return a, false, err
		}
		return a, false, nil
	}
	if in.ProjectID == "" || in.Name == "" {
		return tatnet.V1App{}, false, fmt.Errorf("give app_id, or project_id and name for a new app (whoami lists projects)")
	}
	existing, err := t.findByName(ctx, c, in.ProjectID, in.Name)
	if err != nil {
		return tatnet.V1App{}, false, err
	}
	if existing != nil {
		if st := orDefault(val(existing.SourceType), "git"); st != "upload" {
			return *existing, false, fmt.Errorf("app %q already exists and deploys from %s; choose another name", existing.Name, st)
		}
		if err := backendFromFiles(*existing); err != nil {
			return *existing, false, err
		}
		return *existing, false, nil
	}
	r, err := c.AppsCreateAppWithResponse(ctx, in.ProjectID, tatnet.V1AppCreate{
		Name: in.Name, SourceType: ptr("upload"),
		// Только сайт: платформа пока не собирает бэкенд из папки, и апп
		// другого типа молча упал бы на первой же сборке.
		AppType: ptr("frontend"), Framework: opt(in.Framework),
		BuildCommand: opt(in.BuildCommand), InstallCommand: opt(in.InstallCommand),
		OutputDirectory: opt(in.OutputDirectory),
	})
	if err := check("create app", r, err); err != nil {
		return tatnet.V1App{}, false, err
	}
	return *r.JSON201, true, nil
}

// backendFromFiles — отказ ДО загрузки: платформа не собирает бэкенд из папки
// («Deploying from a folder is not supported for backend apps yet»), и без
// этой проверки модель узнала бы об этом только упавшей сборкой.
func backendFromFiles(a tatnet.V1App) error {
	if val(a.AppType) == "backend" {
		return fmt.Errorf("app %q is a backend service, and backends cannot be deployed from files yet; push the code to a git repository (or a Docker image) and use create_app + deploy_app", a.Name)
	}
	return nil
}
