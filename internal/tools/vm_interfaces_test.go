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

func TestVMInterfacesUseAuthenticatedPublicAPI(t *testing.T) {
	for _, method := range []string{"POST", "DELETE"} {
		t.Run(method, func(t *testing.T) {
			tool := New("https://api.invalid/v1")
			calls := 0
			tool.HTTP.Transport = kafkaRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				want := "/v1/projects/p/vms/v/interfaces"
				if method == "DELETE" {
					want += "/nic"
				}
				if r.Method != method || r.URL.Path != want || r.Header.Get("Authorization") != "Bearer tn_live_test" {
					t.Fatalf("wrong request %s %s", r.Method, r.URL.Path)
				}
				if method == "POST" {
					var body map[string]string
					json.NewDecoder(r.Body).Decode(&body)
					if body["vpc_id"] != "vpc" {
						t.Fatalf("wrong body %v", body)
					}
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"status":"success","interface_id":"nic"}`))}, nil
			})
			handler := auth.RequireBearerToken(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
				return &auth.TokenInfo{Expiration: time.Now().Add(time.Minute), Extra: map[string]any{"principal": authn.Principal{APIKey: "tn_live_test"}}}, nil
			}, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var err error
				if method == "POST" {
					_, _, err = tool.attachVMInterface(r.Context(), nil, AttachVMInterfaceIn{ProjectID: "p", VMID: "v", VPCID: "vpc"})
				} else {
					_, _, err = tool.detachVMInterface(r.Context(), nil, DetachVMInterfaceIn{ProjectID: "p", VMID: "v", InterfaceID: "nic"})
				}
				if err != nil {
					t.Fatal(err)
				}
			}))
			req := httptest.NewRequest("POST", "/mcp", nil)
			req.Header.Set("Authorization", "Bearer tn_live_test")
			handler.ServeHTTP(httptest.NewRecorder(), req)
			if calls != 1 {
				t.Fatalf("sent %d requests", calls)
			}
			if _, _, err := tool.attachVMInterface(context.Background(), nil, AttachVMInterfaceIn{}); err == nil {
				t.Fatal("unauthenticated request accepted")
			}
		})
	}
}
