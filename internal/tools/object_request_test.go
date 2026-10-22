package tools

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/tatnet-ru/tatnet-mcp/internal/authn"
)

func TestAPIRequestAuthenticationAndResponses(t *testing.T) {
	for _, principal := range []authn.Principal{{APIKey: "tn_live_test"}, {Grant: "grant-id"}} {
		name := "key"
		if principal.Grant != "" {
			name = "oauth"
		}
		t.Run(name, func(t *testing.T) {
			for _, tc := range []struct {
				name      string
				status    int
				body      string
				wantError string
			}{
				{"json", 200, `{"id":"resource"}`, ""},
				{"deleted", 204, "", ""},
				{"forbidden", 403, `{"error":{"code":"forbidden","message":"Missing vm:update"}}`, "do not retry"},
				{"conflict", 409, `{"detail":"Backup capture is still in progress"}`, "Backup capture is still in progress"},
				{"rate limited", 429, `{"detail":"Slow down"}`, "retry after 17 seconds"},
				{"redirect", 302, "", "HTTP 302"},
				{"invalid json", 200, "bad json", "invalid API response"},
				{"oversized", 200, `{"data":"` + strings.Repeat("x", 4<<20) + `"}`, "exceeds"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					tool := New("https://api.invalid/v1///")
					tool.InternalSecret = "internal-test"
					tool.HTTP.Transport = kafkaRoundTrip(func(r *http.Request) (*http.Response, error) {
						if r.URL.String() != "https://api.invalid/v1/resource?region=spb-1" {
							t.Fatalf("URL: %s", r.URL)
						}
						if r.Header.Get("Accept") != "application/json" || r.Header.Get("Content-Type") != "application/json" {
							t.Fatal("missing JSON headers")
						}
						raw, _ := io.ReadAll(r.Body)
						if string(raw) != `{"enabled":true}` {
							t.Fatalf("body: %s", raw)
						}
						if principal.Grant != "" {
							if r.Header.Get(authn.GrantHeader) != principal.Grant || r.Header.Get(authn.SecretHeader) != tool.InternalSecret || r.Header.Get("Authorization") != "" {
								t.Fatal("incorrect OAuth delegation")
							}
						} else if r.Header.Get("Authorization") != "Bearer "+principal.APIKey || r.Header.Get(authn.SecretHeader) != "" {
							t.Fatal("incorrect key forwarding")
						}
						return &http.Response{StatusCode: tc.status, Header: http.Header{"Retry-After": {"17"}}, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
					})
					handler := auth.RequireBearerToken(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
						return &auth.TokenInfo{Expiration: time.Now().Add(time.Minute), Extra: map[string]any{"principal": principal}}, nil
					}, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						_, out, err := tool.objectRequest(r.Context(), http.MethodPut, "/resource?region=spb-1", map[string]bool{"enabled": true})
						if tc.wantError != "" {
							if err == nil || !strings.Contains(err.Error(), tc.wantError) {
								t.Fatalf("error: %v, want %q", err, tc.wantError)
							}
						} else {
							if err != nil {
								t.Fatal(err)
							}
							if tc.status == 204 && out.Data == nil {
								t.Fatal("empty deletion must return an object")
							}
							if tc.status == 200 && out.Data["id"] != "resource" {
								t.Fatalf("response: %v", out)
							}
						}
					}))
					req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
					req.Header.Set("Authorization", "Bearer test")
					recorder := httptest.NewRecorder()
					handler.ServeHTTP(recorder, req)
					if recorder.Code != 200 {
						t.Fatalf("request rejected: %d", recorder.Code)
					}
				})
			}
		})
	}
}
