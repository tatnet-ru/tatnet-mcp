package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tatnet-ru/tatnet-mcp/internal/config"
)

const databaseID = "1419b1ae-0aa4-436a-9769-c8fb13d9d770"

func databaseConnect(t *testing.T, endpoint, token string) (*mcp.ClientSession, error) {
	t.Helper()
	return mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(context.Background(),
		&mcp.StreamableClientTransport{Endpoint: endpoint + "/supabase/mcp", HTTPClient: &http.Client{Transport: bearer{token}}}, nil)
}

func TestSupabaseOAuthMetadataAndAudienceSeparation(t *testing.T) {
	_, hydra, srv := setupOAuth(t)
	req, _ := http.NewRequest("GET", srv.URL+"/.well-known/oauth-protected-resource/supabase/mcp", nil)
	req.Host = "mcp.test"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	json.NewDecoder(resp.Body).Decode(&meta)
	resp.Body.Close()
	if meta["resource"] != publicURL+"/supabase/mcp" {
		t.Fatalf("wrong resource: %v", meta)
	}
	// Verify both directions; also after a successful request populated the cache.
	appToken := hydra.token(t, nil)
	dbToken := hydra.token(t, func(c jwt.MapClaims) { c["aud"] = []string{publicURL + "/supabase/mcp"} })
	app, err := connect(t, srv.URL, appToken)
	if err != nil {
		t.Fatal(err)
	}
	app.Close()
	if session, err := databaseConnect(t, srv.URL, appToken); err == nil {
		session.Close()
		t.Fatal("app audience accepted for database endpoint")
	}
	db, err := databaseConnect(t, srv.URL, dbToken)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if session, err := connect(t, srv.URL, dbToken); err == nil {
		session.Close()
		t.Fatal("database audience accepted for app endpoint")
	}
	if session, err := databaseConnect(t, srv.URL, ""); err == nil {
		session.Close()
		t.Fatal("unauthenticated connection accepted")
	}
}

func TestSupabaseToolsForwardGrantAndDoNotRetrySQL(t *testing.T) {
	hydra := newFakeHydra(t)
	var sqlCalls atomic.Int32
	var deny, sqlError atomic.Bool
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-TatNet-Grant") != liveGrant || r.Header.Get("X-TatNet-Internal-Secret") != internalSecret {
			t.Error("OAuth token leaked or grant headers missing")
			http.Error(w, "unauthorized", 401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/account" {
			io.WriteString(w, `{"account_id":"a1","key_id":"k1"}`)
			return
		}
		if deny.Load() {
			w.WriteHeader(403)
			io.WriteString(w, `{"error":{"message":"grant revoked or SQL denied"}}`)
			return
		}
		if r.URL.Path == "/supabase" {
			io.WriteString(w, `{"data":[{"id":"`+databaseID+`","name":"DB","status":"active","read_only":false}],"limit":50,"offset":0,"count":1}`)
			return
		}
		if r.URL.Path == "/supabase/"+databaseID+"/sql" {
			sqlCalls.Add(1)
		}
		if sqlError.Load() {
			io.WriteString(w, `{"ok":false,"results":[],"error":{"message":"bad SQL","sqlstate":"42601","statement_index":0}}`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/sql") {
			w.WriteHeader(503)
			io.WriteString(w, `{"error":{"message":"timeout"}}`)
			return
		}
		if r.URL.Path != "/supabase/"+databaseID+"/query" {
			t.Error("unexpected path", r.URL.Path)
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		if body["max_rows"] != float64(7) || body["timeout_ms"] != float64(1000) {
			t.Error("limits not forwarded", body)
		}
		io.WriteString(w, `{"ok":true,"results":[{"command_tag":"SELECT 1","rows_affected":1,"columns":["value"],"rows":[["untrusted data"]]}]}`)
	}))
	defer api.Close()
	srv := httptest.NewServer(Routes(config.Config{APIBaseURL: api.URL, PublicURL: publicURL, OIDCIssuer: hydra.srv.URL,
		OIDCJWKSURL: hydra.srv.URL + "/jwks", InternalSecret: internalSecret}, slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer srv.Close()
	token := hydra.token(t, func(c jwt.MapClaims) { c["aud"] = []string{publicURL + "/supabase/mcp"} })
	s, err := databaseConnect(t, srv.URL, token)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	list, err := s.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Tools) != 3 {
		t.Fatalf("expected only 3 database tools: %v", list.Tools)
	}
	for _, tool := range list.Tools {
		if tool.Name == "execute_sql" && (tool.Annotations.ReadOnlyHint || tool.Annotations.DestructiveHint == nil || !*tool.Annotations.DestructiveHint || tool.Annotations.IdempotentHint) {
			t.Fatal("unsafe SQL annotations")
		}
	}
	out, _, bad := call(t, s, "list_databases", map[string]any{})
	if bad || len(out["data"].([]any)) != 1 {
		t.Fatal("list failed", out)
	}
	args := map[string]any{"database_id": databaseID, "sql": "SELECT 1", "max_rows": 7, "timeout_ms": 1000}
	out, _, bad = call(t, s, "query_database", args)
	if bad || out["ok"] != true {
		t.Fatal("query failed", out)
	}
	_, text, bad := call(t, s, "execute_sql", args)
	if !bad || sqlCalls.Load() != 1 || !strings.Contains(text, "do not automatically retry") {
		t.Fatal("unknown outcome must fail without retry", text, sqlCalls.Load())
	}
	sqlError.Store(true)
	out, text, bad = call(t, s, "query_database", args)
	if !bad || (out["error"] == nil && !strings.Contains(text, "bad SQL")) {
		t.Fatal("SQL error details must reach the model", out, text)
	}
	deny.Store(true)
	_, _, bad = call(t, s, "query_database", args)
	if !bad {
		t.Fatal("API authorization denial was ignored after authentication cache")
	}
}

// Every registered resource must keep its own metadata URL and reject unknown
// hosts, including aliases. The route registry also drives token audiences.
func TestMCPResourceRegistry(t *testing.T) {
	cfg := config.Config{PublicURL: publicURL, ExtraPublicURLs: []string{"https://mcp.old"}, OIDCIssuer: "https://auth.test"}
	handler := Routes(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, pub := range cfg.PublicURLs() {
		for _, resource := range mcpResources {
			host := publicHost(pub)
			metadataPath := "/.well-known/oauth-protected-resource" + resource.path
			req := httptest.NewRequest("GET", pub+metadataPath, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			var meta map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &meta); err != nil {
				t.Fatal(err)
			}
			if rec.Code != 200 || meta["resource"] != pub+resource.path {
				t.Fatal("incorrect resource metadata", rec.Code, meta)
			}
			if resources(cfg)[host+resource.path] != meta["resource"] {
				t.Fatal("discovery and verifier disagree")
			}
			req = httptest.NewRequest("POST", pub+resource.path, strings.NewReader(`{}`))
			rec = httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != 401 || !strings.Contains(rec.Header().Get("WWW-Authenticate"), pub+metadataPath) {
				t.Fatal("incorrect OAuth discovery challenge", rec.Code, rec.Header())
			}
			for _, path := range []string{resource.path, metadataPath} {
				req = httptest.NewRequest("GET", "https://untrusted.test"+path, nil)
				rec = httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				if rec.Code != 421 {
					t.Fatal("unknown host accepted", path, rec.Code)
				}
			}
		}
	}
}
