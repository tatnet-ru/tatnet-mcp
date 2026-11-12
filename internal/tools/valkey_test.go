package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tatnet-ru/tatnet-mcp/internal/authn"
)

func TestValkeyTLSUsesCallerAPIAndPreservesReceipt(t *testing.T) {
	for _, status := range []int{200, 409, 403} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "PUT" || r.URL.Path != "/projects/p/valkey-clusters/c/params" {
					t.Errorf("wrong target: %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer tn_live_test" {
					t.Error("did not forward caller key")
				}
				var body map[string]any
				json.NewDecoder(r.Body).Decode(&body)
				if body["tls_mode"] != "required" || len(body) != 1 {
					t.Errorf("body %v", body)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if status == 200 {
					fmt.Fprint(w, `{"tls_mode":"required","parameters_applied":false}`)
				} else {
					fmt.Fprint(w, `{"detail":"nodes need replacement"}`)
				}
			}))
			defer api.Close()
			tools := &Tools{APIBase: api.URL, HTTP: api.Client()}
			middleware := auth.RequireBearerToken(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
				return &auth.TokenInfo{Expiration: time.Now().Add(time.Hour), Extra: map[string]any{"principal": authn.Principal{APIKey: "tn_live_test"}}}, nil
			}, nil)
			called := false
			h := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				out, err := tools.updateValkeyTLS(r.Context(), UpdateValkeyTLSIn{ValkeyTargetIn: ValkeyTargetIn{ProjectID: "p", ClusterID: "c"}, TLSMode: "required"})
				if status == 200 {
					if err != nil || out.Cluster["parameters_applied"] != false {
						t.Fatalf("lost pending state: %+v %v", out, err)
					}
				} else if err == nil || !strings.Contains(err.Error(), "nodes need replacement") {
					t.Fatalf("lost API rejection: %v", err)
				}
			}))
			req := httptest.NewRequest("POST", "/", nil)
			req.Header.Set("Authorization", "Bearer test")
			h.ServeHTTP(httptest.NewRecorder(), req)
			if !called {
				t.Fatal("test authentication did not reach handler")
			}
		})
	}
}

func TestValkeyToolsRegister(t *testing.T) {
	// Registration compiles every input/output schema, including embedded IDs.
	s := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	(&Tools{}).registerValkey(s)
}

func TestValkeyCreateOmitsUnsetOptionalFields(t *testing.T) {
	for _, mode := range []string{"", "preferred", "disabled"} {
		data, err := valkeyCreateBody(CreateValkeyIn{ProjectID: "p", Name: "cache", PlanID: "plan", TLSMode: mode})
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		if err := json.Unmarshal(data, &body); err != nil {
			t.Fatal(err)
		}
		if _, ok := body["ip_allowlist"]; ok {
			t.Fatal("unset allowlist must be omitted, not null", string(data))
		}
		if body["name"] != "cache" || body["plan_id"] != "plan" {
			t.Fatal(string(data))
		}
		if mode != "" && body["tls_mode"] != mode {
			t.Fatal(string(data))
		}
		if _, ok := body["project_id"]; ok {
			t.Fatal("project id belongs in path")
		}
	}
}
