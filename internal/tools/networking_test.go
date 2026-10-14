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
