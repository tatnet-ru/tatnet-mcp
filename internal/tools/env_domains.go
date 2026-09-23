package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tatnet-ru/tatnet-go/tatnet"
)

// ---------- переменные окружения ----------

type EnvVarView struct {
	Name     string `json:"name"`
	IsSecret bool   `json:"is_secret"`
	// Значение секрета НЕ отдаётся никогда: контекст модели логируется у
	// провайдера, и секрет, попавший туда, считается утёкшим.
	Value string `json:"value,omitempty" jsonschema:"value of a non-secret variable; secret values are never returned"`
}

type ListEnvIn struct {
	AppID string `json:"app_id"`
}

type ListEnvOut struct {
	Vars []EnvVarView `json:"vars"`
}

type SetEnvVar struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Secret *bool  `json:"secret,omitempty" jsonschema:"store as a secret: never shown again (default true)"`
}

type SetEnvIn struct {
	AppID string      `json:"app_id"`
	Vars  []SetEnvVar `json:"vars"`
}

type SetEnvOut struct {
	Set  []string `json:"set"`
	Note string   `json:"note"`
}

type DeleteEnvIn struct {
	AppID string `json:"app_id"`
	Name  string `json:"name"`
}

type DeleteEnvOut struct {
	Deleted string `json:"deleted"`
	Note    string `json:"note"`
}

const envNote = "Running replicas restart with the new values. Variables read at BUILD time (NEXT_PUBLIC_*, VITE_*) take effect only after a new deploy."

func (t *Tools) listEnv(ctx context.Context, c *tatnet.ClientWithResponses, appID string) ([]tatnet.V1EnvVar, error) {
	r, err := c.AppsListEnvVarsByIdWithResponse(ctx, appID, &tatnet.AppsListEnvVarsByIdParams{Limit: ptr(200)})
	if err := check("list env", r, err); err != nil {
		return nil, err
	}
	return r.JSON200.Data, nil
}

func (t *Tools) registerEnv(s *mcp.Server) {
	add(s, &mcp.Tool{
		Name:        "list_env",
		Description: "List an app's environment variables. Secret values are never returned.",
		Annotations: readOnly("List environment variables"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ListEnvIn) (*mcp.CallToolResult, ListEnvOut, error) {
		out := ListEnvOut{Vars: []EnvVarView{}}
		c, err := t.client(ctx)
		if err != nil {
			return nil, out, err
		}
		vars, err := t.listEnv(ctx, c, in.AppID)
		if err != nil {
			return nil, out, err
		}
		for _, v := range vars {
			ev := EnvVarView{Name: v.Name, IsSecret: v.IsSecret}
			if !v.IsSecret {
				ev.Value = v.Value
			}
			out.Vars = append(out.Vars, ev)
		}
		return nil, out, nil
	})

	add(s, &mcp.Tool{
		Name:        "set_env",
		Description: "Create or update environment variables of an app (by name). Values are stored as secrets unless secret=false.",
		Annotations: additive("Set environment variables", true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in SetEnvIn) (*mcp.CallToolResult, SetEnvOut, error) {
		out := SetEnvOut{Set: []string{}, Note: envNote}
		if len(in.Vars) == 0 {
			return nil, out, fmt.Errorf("vars is empty")
		}
		c, err := t.client(ctx)
		if err != nil {
			return nil, out, err
		}
		for _, v := range in.Vars {
			secret := true
			if v.Secret != nil {
				secret = *v.Secret
			}
			r, err := c.AppsAddEnvVarByIdWithResponse(ctx, in.AppID, tatnet.V1EnvVarCreate{Name: v.Name, Value: v.Value, IsSecret: ptr(secret)})
			if err := check("set "+v.Name, r, err); err != nil {
				// Частичный успех называется, а не прячется за ошибкой.
				if len(out.Set) > 0 {
					return nil, out, fmt.Errorf("%v (already set: %s)", err, strings.Join(out.Set, ", "))
				}
				return nil, out, err
			}
			out.Set = append(out.Set, v.Name)
		}
		return nil, out, nil
	})

	add(s, &mcp.Tool{
		Name:        "delete_env",
		Description: "Delete an environment variable of an app by name.",
		Annotations: destructive("Delete environment variable"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in DeleteEnvIn) (*mcp.CallToolResult, DeleteEnvOut, error) {
		var out DeleteEnvOut
		c, err := t.client(ctx)
		if err != nil {
			return nil, out, err
		}
		vars, err := t.listEnv(ctx, c, in.AppID)
		if err != nil {
			return nil, out, err
		}
		for _, v := range vars {
			if v.Name == in.Name {
				r, err := c.AppsDeleteEnvVarByIdWithResponse(ctx, in.AppID, v.Id)
				if err := check("delete env", r, err); err != nil {
					return nil, out, err
				}
				out.Deleted, out.Note = v.Name, envNote
				return nil, out, nil
			}
		}
		return nil, out, fmt.Errorf("the app has no variable %q", in.Name)
	})
}

// ---------- домены ----------

type DomainView struct {
	ID            string `json:"id"`
	Domain        string `json:"domain"`
	IsDefault     bool   `json:"is_default"`
	Status        string `json:"status" jsonschema:"pending (waiting for DNS) | active | error ..."`
	DNSConfigured bool   `json:"dns_configured"`
	TargetIP      string `json:"target_ip,omitempty" jsonschema:"where the domain's A record must point"`
	Error         string `json:"error,omitempty"`
}

func domainView(d tatnet.V1AppDomain) DomainView {
	return DomainView{
		ID: d.Id, Domain: d.Domain, IsDefault: val(d.IsDefault), Status: d.Status,
		DNSConfigured: val(d.DnsConfigured), TargetIP: val(d.TargetIp), Error: val(d.Error),
	}
}

func (t *Tools) listDomains(ctx context.Context, c *tatnet.ClientWithResponses, appID string) ([]DomainView, error) {
	r, err := c.AppsListDomainsByIdWithResponse(ctx, appID, &tatnet.AppsListDomainsByIdParams{Limit: ptr(200)})
	if err := check("list domains", r, err); err != nil {
		return nil, err
	}
	out := []DomainView{}
	for _, d := range r.JSON200.Data {
		// Домены конкретных сборок (preview по коммиту) — не адрес аппа.
		if typ := val(d.Type); typ != "" && typ != "current" {
			continue
		}
		out = append(out, domainView(d))
	}
	return out, nil
}

type ListDomainsIn struct {
	AppID string `json:"app_id"`
}

type ListDomainsOut struct {
	Domains []DomainView `json:"domains"`
}

type AddDomainIn struct {
	AppID  string `json:"app_id"`
	Domain string `json:"domain" jsonschema:"e.g. shop.example.ru"`
}

type AddDomainOut struct {
	Domain       DomainView `json:"domain"`
	Instructions string     `json:"instructions"`
}

type RemoveDomainIn struct {
	AppID  string `json:"app_id"`
	Domain string `json:"domain" jsonschema:"the domain name to detach"`
}

type RemoveDomainOut struct {
	Removed string `json:"removed"`
}

func (t *Tools) registerDomains(s *mcp.Server) {
	add(s, &mcp.Tool{
		Name:        "list_domains",
		Description: "List an app's domains with DNS and certificate status.",
		Annotations: readOnly("List domains"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in ListDomainsIn) (*mcp.CallToolResult, ListDomainsOut, error) {
		var out ListDomainsOut
		c, err := t.client(ctx)
		if err != nil {
			return nil, out, err
		}
		out.Domains, err = t.listDomains(ctx, c, in.AppID)
		return nil, out, err
	})

	add(s, &mcp.Tool{
		Name: "add_domain",
		Description: "Attach a custom domain to an app. If the domain's DNS zone is managed by TatNet the record is created automatically; " +
			"otherwise the user must create an A record to target_ip. HTTPS certificate is issued automatically once DNS resolves.",
		Annotations: additive("Add domain", true),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in AddDomainIn) (*mcp.CallToolResult, AddDomainOut, error) {
		var out AddDomainOut
		c, err := t.client(ctx)
		if err != nil {
			return nil, out, err
		}
		name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(in.Domain), "."))
		r, err := c.AppsAddDomainByIdWithResponse(ctx, in.AppID, tatnet.V1DomainCreate{Domain: name})
		if err := check("add domain", r, err); err != nil {
			return nil, out, err
		}
		out.Domain = domainView(*r.JSON201)
		switch {
		case out.Domain.DNSConfigured:
			out.Instructions = "DNS is configured automatically (the zone is managed by TatNet). The certificate is issued within minutes."
		case out.Domain.TargetIP != "":
			out.Instructions = fmt.Sprintf("Ask the user to create an A record %s -> %s at their DNS provider. TatNet checks DNS periodically and issues the certificate once it resolves.", name, out.Domain.TargetIP)
		default:
			out.Instructions = "The domain is attached; check list_domains for its status."
		}
		return nil, out, nil
	})

	add(s, &mcp.Tool{
		Name:        "remove_domain",
		Description: "Detach a custom domain from an app. The app stops answering on it.",
		Annotations: destructive("Remove domain"),
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in RemoveDomainIn) (*mcp.CallToolResult, RemoveDomainOut, error) {
		var out RemoveDomainOut
		c, err := t.client(ctx)
		if err != nil {
			return nil, out, err
		}
		domains, err := t.listDomains(ctx, c, in.AppID)
		if err != nil {
			return nil, out, err
		}
		name := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(in.Domain), "."))
		for _, d := range domains {
			if d.Domain != name {
				continue
			}
			if d.IsDefault {
				return nil, out, fmt.Errorf("%s is the app's default address and cannot be removed", name)
			}
			r, err := c.AppsRemoveDomainByIdWithResponse(ctx, in.AppID, d.ID)
			if err := check("remove domain", r, err); err != nil {
				return nil, out, err
			}
			out.Removed = name
			return nil, out, nil
		}
		return nil, out, fmt.Errorf("the app has no domain %q", name)
	})
}
