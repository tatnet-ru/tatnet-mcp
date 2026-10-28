package tools

import (
	"context"
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/tatnet-ru/tatnet-mcp/internal/authn"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoadBalancerReservationUsesPublicAPI(t *testing.T) {
	for _, create := range []bool{false, true} {
		t.Run(map[bool]string{false: "get", true: "create"}[create], func(t *testing.T) {
			tool := New("https://api.invalid/v1")
			called := false
			tool.HTTP.Transport = kafkaRoundTrip(func(r *http.Request) (*http.Response, error) {
				called = true
				wantMethod, wantPath := "GET", "/v1/projects/p%2Fid/load-balancers/lb"
				if create {
					wantMethod, wantPath = "POST", "/v1/projects/p%2Fid/load-balancers"
				}
				if r.Method != wantMethod || r.URL.EscapedPath() != wantPath || r.Header.Get("Authorization") != "Bearer tn_live_test" {
					t.Fatalf("unexpected request %s %s", r.Method, r.URL)
				}
				if create {
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Fatal(err)
					}
					if body["floating_ip_id"] != "reserved" || body["vpc_id"] != "vpc" || body["node_count"] != float64(2) {
						t.Fatalf("wrong body %v", body)
					}
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"lb","floating_ip_id":"reserved"}`))}, nil
			})
			handler := auth.RequireBearerToken(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
				return &auth.TokenInfo{Expiration: time.Now().Add(time.Minute), Extra: map[string]any{"principal": authn.Principal{APIKey: "tn_live_test"}}}, nil
			}, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var out ObjectOut
				var err error
				if create {
					_, out, err = tool.createLoadBalancer(r.Context(), nil, CreateLoadBalancerIn{ProjectID: "p/id", Name: "web", VPCID: "vpc", FloatingIPID: "reserved", NodeCount: 2})
				} else {
					_, out, err = tool.getLoadBalancer(r.Context(), nil, GetLoadBalancerIn{ProjectID: "p/id", LoadBalancerID: "lb"})
				}
				if err != nil || out.Data["floating_ip_id"] != "reserved" {
					t.Fatalf("result %v %v", out, err)
				}
			}))
			req := httptest.NewRequest("POST", "/mcp", nil)
			req.Header.Set("Authorization", "Bearer tn_live_test")
			handler.ServeHTTP(httptest.NewRecorder(), req)
			if !called {
				t.Fatal("API not called")
			}
		})
	}
}
func TestLoadBalancerCreateRequiresPrincipal(t *testing.T) {
	tool := New("https://api.invalid/v1")
	tool.HTTP.Transport = kafkaRoundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("unauthenticated request escaped")
		return nil, nil
	})
	if _, _, err := tool.createLoadBalancer(context.Background(), nil, CreateLoadBalancerIn{}); err == nil {
		t.Fatal("missing principal accepted")
	}
}

func TestRetainLoadBalancerIPUsesExactVIPAndEscapedResource(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusConflict} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			tool := New("https://api.invalid/v1")
			called := false
			tool.HTTP.Transport = kafkaRoundTrip(func(r *http.Request) (*http.Response, error) {
				called = true
				if r.Method != "POST" || r.URL.EscapedPath() != "/v1/projects/p%2Fid/load-balancers/lb%2Fid/retain-ip" || r.Header.Get("Authorization") != "Bearer tn_live_test" {
					t.Fatalf("unexpected request %s %s", r.Method, r.URL)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if len(body) != 1 || body["expected_vip"] != "198.51.100.42" {
					t.Fatalf("wrong body %v", body)
				}
				response := `{"id":"lb","vip":"198.51.100.42","floating_ip_id":"allocation","ip_billing_included":true}`
				if status == http.StatusConflict {
					response = `{"error":{"code":"vip_changed","message":"VIP changed"}}`
				}
				return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(response))}, nil
			})
			handler := auth.RequireBearerToken(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
				return &auth.TokenInfo{Expiration: time.Now().Add(time.Minute), Extra: map[string]any{"principal": authn.Principal{APIKey: "tn_live_test"}}}, nil
			}, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, out, err := tool.retainLoadBalancerIP(r.Context(), nil, RetainLoadBalancerIPIn{ProjectID: "p/id", LoadBalancerID: "lb/id", ExpectedVIP: "198.51.100.42"})
				if status == http.StatusConflict {
					if err == nil {
						t.Fatal("409 was hidden")
					}
					return
				}
				if err != nil || out.Data["floating_ip_id"] != "allocation" || out.Data["vip"] != "198.51.100.42" {
					t.Fatalf("result %v %v", out, err)
				}
			}))
			req := httptest.NewRequest("POST", "/mcp", nil)
			req.Header.Set("Authorization", "Bearer tn_live_test")
			handler.ServeHTTP(httptest.NewRecorder(), req)
			if !called {
				t.Fatal("API not called")
			}
		})
	}
	tool := New("https://api.invalid/v1")
	tool.HTTP.Transport = kafkaRoundTrip(func(*http.Request) (*http.Response, error) { t.Fatal("missing principal escaped"); return nil, nil })
	if _, _, err := tool.retainLoadBalancerIP(context.Background(), nil, RetainLoadBalancerIPIn{}); err == nil {
		t.Fatal("missing principal accepted")
	}
}
