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

func TestRenameVpcUsesAuthenticatedMetadataPatch(t *testing.T) {
	tool := New("https://api.invalid/v1")
	tool.HTTP.Transport = kafkaRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method != "PATCH" || r.URL.Path != "/v1/vpcs/vpc-id" || r.Header.Get("Authorization") != "Bearer tn_live_test" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body) != 1 || body["name"] != "New network" {
			t.Fatalf("unexpected body %v", body)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"vpc-id","name":"New network"}`))}, nil
	})
	handler := auth.RequireBearerToken(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return &auth.TokenInfo{Expiration: time.Now().Add(time.Minute), Extra: map[string]any{"principal": authn.Principal{APIKey: "tn_live_test"}}}, nil
	}, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, out, err := tool.renameVpc(r.Context(), nil, RenameVpcIn{VPCID: "vpc-id", Name: "New network"})
		if err != nil || out.Data["name"] != "New network" {
			t.Fatalf("result %v %v", out, err)
		}
	}))
	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer tn_live_test")
	handler.ServeHTTP(httptest.NewRecorder(), req)
}
func TestRenameVpcRequiresPrincipal(t *testing.T) {
	tool := New("https://api.invalid/v1")
	tool.HTTP.Transport = kafkaRoundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("unauthenticated request escaped")
		return nil, nil
	})
	if _, _, err := tool.renameVpc(context.Background(), nil, RenameVpcIn{VPCID: "v", Name: "new"}); err == nil {
		t.Fatal("missing principal accepted")
	}
}

func TestDisableNatGatewayForwardsRetentionAndAllocationGuard(t *testing.T) {
	for _, retain := range []bool{false, true} {
		t.Run(map[bool]string{false: "release", true: "retain"}[retain], func(t *testing.T) {
			tool := New("https://api.invalid/v1")
			called := false
			tool.HTTP.Transport = kafkaRoundTrip(func(r *http.Request) (*http.Response, error) {
				called = true
				want := map[bool]string{false: "false", true: "true"}[retain]
				if r.Method != "DELETE" || r.URL.EscapedPath() != "/v1/vpcs/vpc%2Fid/nat-gateway" || r.URL.Query().Get("retain_ip") != want || r.URL.Query().Get("expected_fip_id") != "fip-id" || r.Header.Get("Authorization") != "Bearer tn_live_test" {
					t.Fatalf("unexpected request %s %s", r.Method, r.URL)
				}
				return &http.Response{StatusCode: 202, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"fip_id":"fip-id","status":"detaching"}`))}, nil
			})
			handler := auth.RequireBearerToken(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
				return &auth.TokenInfo{Expiration: time.Now().Add(time.Minute), Extra: map[string]any{"principal": authn.Principal{APIKey: "tn_live_test"}}}, nil
			}, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, out, err := tool.disableNatGateway(r.Context(), nil, DisableNatGatewayIn{VPCID: "vpc/id", RetainIP: retain, ExpectedFIPID: "fip-id"})
				if err != nil || out.Data["status"] != "detaching" {
					t.Fatalf("result %v %v", out, err)
				}
			}))
			req := httptest.NewRequest("POST", "/mcp", nil)
			req.Header.Set("Authorization", "Bearer tn_live_test")
			handler.ServeHTTP(httptest.NewRecorder(), req)
			if !called {
				t.Fatal("API was not called")
			}
		})
	}
}

func TestDisableNatGatewayRequiresPrincipal(t *testing.T) {
	tool := New("https://api.invalid/v1")
	tool.HTTP.Transport = kafkaRoundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("unauthenticated request escaped")
		return nil, nil
	})
	if _, _, err := tool.disableNatGateway(context.Background(), nil, DisableNatGatewayIn{VPCID: "v"}); err == nil {
		t.Fatal("missing principal accepted")
	}
}

func TestEnableNatGatewayUsesAuthenticatedAPI(t *testing.T) {
	for _, fipID := range []string{"", "retained-fip"} {
		t.Run("fip="+fipID, func(t *testing.T) {
			tool := New("https://api.invalid/v1")
			called := false
			tool.HTTP.Transport = kafkaRoundTrip(func(r *http.Request) (*http.Response, error) {
				called = true
				if r.Method != "POST" || r.URL.EscapedPath() != "/v1/vpcs/vpc%2Fid/nat-gateway" || r.Header.Get("Authorization") != "Bearer tn_live_test" {
					t.Fatalf("unexpected request %s %s", r.Method, r.URL)
				}
				var body map[string]string
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if body["fip_id"] != fipID || len(body) != map[bool]int{true: 0, false: 1}[fipID == ""] {
					t.Fatalf("unexpected body %v", body)
				}
				return &http.Response{StatusCode: 201, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"fip_id":"retained-fip","status":"attaching"}`))}, nil
			})
			handler := auth.RequireBearerToken(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
				return &auth.TokenInfo{Expiration: time.Now().Add(time.Minute), Extra: map[string]any{"principal": authn.Principal{APIKey: "tn_live_test"}}}, nil
			}, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, out, err := tool.enableNatGateway(r.Context(), nil, EnableNatGatewayIn{VPCID: "vpc/id", FIPID: fipID})
				if err != nil || out.Data["status"] != "attaching" {
					t.Fatalf("result %v %v", out, err)
				}
			}))
			req := httptest.NewRequest("POST", "/mcp", nil)
			req.Header.Set("Authorization", "Bearer tn_live_test")
			handler.ServeHTTP(httptest.NewRecorder(), req)
			if !called {
				t.Fatal("API was not called")
			}
		})
	}
}

func TestEnableNatGatewayRequiresPrincipal(t *testing.T) {
	tool := New("https://api.invalid/v1")
	tool.HTTP.Transport = kafkaRoundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("unauthenticated request escaped")
		return nil, nil
	})
	if _, _, err := tool.enableNatGateway(context.Background(), nil, EnableNatGatewayIn{VPCID: "v"}); err == nil {
		t.Fatal("missing principal accepted")
	}
}
