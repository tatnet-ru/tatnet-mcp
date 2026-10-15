package tools

import (
	"context"
	"encoding/json"
	"github.com/tatnet-ru/tatnet-mcp/internal/authn"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestKubernetesTLS(t *testing.T) {
	const project = "11111111-1111-4111-8111-111111111111"
	const cluster = "22222222-2222-4222-8222-222222222222"
	for _, grant := range []bool{false, true} {
		t.Run(map[bool]string{false: "key", true: "grant"}[grant], func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "PATCH" || r.URL.Path != "/v1/projects/"+project+"/kubernetes-clusters/"+cluster+"/settings" {
					t.Errorf("wrong request %s %s", r.Method, r.URL.Path)
				}
				if grant {
					if r.Header.Get(authn.GrantHeader) != "grant-id" || r.Header.Get(authn.SecretHeader) != "secret" || r.Header.Get("Authorization") != "" {
						t.Error("incorrect grant authentication")
					}
				} else if r.Header.Get("Authorization") != "Bearer tn_live_test" {
					t.Error("missing API key")
				}
				var payload map[string]string
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Error(err)
				}
				if payload["tls_min_version"] != "VersionTLS13" {
					t.Errorf("wrong payload %v", payload)
				}
				_, _ = w.Write([]byte(`{"id":"` + cluster + `","tls_min_version":"VersionTLS13"}`))
			}))
			defer srv.Close()
			tool := New(srv.URL + "/v1")
			tool.InternalSecret = "secret"
			p := authn.Principal{APIKey: "tn_live_test"}
			if grant {
				p = authn.Principal{Grant: "grant-id"}
			}
			out, err := tool.setKubernetesTLS(context.Background(), p, KubernetesSettingsIn{ProjectID: project, ClusterID: cluster, TLSMinVersion: "VersionTLS13"})
			if err != nil || out.TLSMinVersion != "VersionTLS13" {
				t.Fatalf("%+v, %v", out, err)
			}
		})
	}
	for _, code := range []int{403, 409, 422} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(code)
			_, _ = w.Write([]byte(`{"detail":"rejected"}`))
		}))
		_, err := New(srv.URL).setKubernetesTLS(context.Background(), authn.Principal{APIKey: "test"}, KubernetesSettingsIn{ProjectID: project, ClusterID: cluster, TLSMinVersion: "invalid"})
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), "rejected") {
			t.Fatalf("HTTP %d: %v", code, err)
		}
	}
}
