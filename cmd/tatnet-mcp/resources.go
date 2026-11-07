package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tatnet-ru/tatnet-mcp/internal/config"
	"github.com/tatnet-ru/tatnet-mcp/internal/tools"
)

type mcpResource struct {
	path, name, implementation, instructions string
	register                                 func(*tools.Tools, *mcp.Server)
}

// The same definitions drive routing, discovery, token audiences and DCR.
var mcpResources = []mcpResource{
	{path: "/mcp", name: "TatNet", implementation: "tatnet", instructions: tools.Instructions, register: (*tools.Tools).Register},
	{path: "/supabase/mcp", name: "TatNet Supabase", implementation: "tatnet-supabase", instructions: tools.SupabaseInstructions, register: (*tools.Tools).RegisterSupabase},
}

func mountMCPResource(mux *http.ServeMux, cfg config.Config, log *slog.Logger, resource mcpResource, server *mcp.Server, verify auth.TokenVerifier) {
	// The platform's loopback proxy needs SDK localhost protection disabled.
	// Public Host checks remain mandatory for every resource and metadata route.
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true, Logger: log, DisableLocalhostProtection: true})
	metadataPath := "/.well-known/oauth-protected-resource" + resource.path
	byHost := map[string]http.Handler{}
	metadata := map[string]map[string]any{}
	for _, pub := range cfg.PublicURLs() {
		host := publicHost(pub)
		// API keys can be non-expiring; OAuth JWTs still require exp in verifier.
		opts := &auth.RequireBearerTokenOptions{AllowMissingExpiration: true}
		if cfg.OIDCIssuer != "" {
			opts.ResourceMetadataURL = pub + metadataPath
			metadata[host] = map[string]any{
				"resource":                 pub + resource.path,
				"authorization_servers":    []string{cfg.OIDCIssuer},
				"scopes_supported":         []string{"openid", "offline_access"},
				"bearer_methods_supported": []string{"header"},
				"resource_name":            resource.name,
			}
		}
		byHost[host] = auth.RequireBearerToken(verify, opts)(handler)
	}
	mux.HandleFunc(resource.path, func(w http.ResponseWriter, r *http.Request) {
		h, ok := byHost[strings.ToLower(r.Host)]
		if !ok {
			http.Error(w, "unknown host", http.StatusMisdirectedRequest)
			return
		}
		h.ServeHTTP(w, r)
	})
	if cfg.OIDCIssuer == "" {
		return
	}
	serveMetadata := func(w http.ResponseWriter, r *http.Request) {
		value, ok := metadata[strings.ToLower(r.Host)]
		if !ok {
			http.Error(w, "unknown host", http.StatusMisdirectedRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		_ = json.NewEncoder(w).Encode(value)
	}
	mux.HandleFunc("GET "+metadataPath, serveMetadata)
	// Preserve the original root discovery alias for the app resource.
	if resource.path == "/mcp" {
		mux.HandleFunc("GET /.well-known/oauth-protected-resource", serveMetadata)
	}
}

func publicHost(publicURL string) string {
	parsed, err := url.Parse(publicURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Host)
}

func resources(cfg config.Config) map[string]string {
	result := map[string]string{}
	for _, pub := range cfg.PublicURLs() {
		for _, resource := range mcpResources {
			result[publicHost(pub)+resource.path] = pub + resource.path
		}
	}
	return result
}
