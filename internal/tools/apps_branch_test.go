package tools

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/tatnet-ru/tatnet-mcp/internal/authn"
)

func TestSetAppBranchUsesCallerAuthorizationAndDoesNotDeploy(t *testing.T) {
	for _, status := range []int{200, 403, 422} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			tool := New("https://api.invalid/v1")
			calls := 0
			tool.HTTP.Transport = kafkaRoundTrip(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.Method != "PATCH" || r.URL.Path != "/v1/apps/control" || r.Header.Get("Authorization") != "Bearer tn_live_test" {
					t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				if len(body) != 1 || body["branch"] != "main" {
					t.Fatalf("unexpected patch %v", body)
				}
				bodyText := `{"detail":"branch update rejected"}`
				if status == 200 {
					bodyText = `{"id":"control","name":"inference-control","project_id":"project","status":"active","source_type":"git","branch":"main"}`
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(bodyText))}, nil
			})
			handler := auth.RequireBearerToken(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
				return &auth.TokenInfo{Expiration: time.Now().Add(time.Minute), Extra: map[string]any{"principal": authn.Principal{APIKey: "tn_live_test"}}}, nil
			}, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, out, err := tool.setAppBranch(r.Context(), nil, SetAppBranchIn{AppID: "control", Branch: "main"})
				if status == 200 {
					if err != nil || out.App.Branch != "main" {
						t.Fatalf("result %+v %v", out, err)
					}
				} else if err == nil || !strings.Contains(err.Error(), "branch update rejected") {
					t.Fatalf("API rejection lost: %v", err)
				}
			}))
			req := httptest.NewRequest("POST", "/mcp", nil)
			req.Header.Set("Authorization", "Bearer tn_live_test")
			handler.ServeHTTP(httptest.NewRecorder(), req)
			if calls != 1 {
				t.Fatalf("unexpected number of requests: %d", calls)
			}
		})
	}
}

func TestSetAppBranchRequiresPrincipal(t *testing.T) {
	tool := New("https://api.invalid/v1")
	tool.HTTP.Transport = kafkaRoundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("unauthenticated request escaped")
		return nil, nil
	})
	if _, _, err := tool.setAppBranch(context.Background(), nil, SetAppBranchIn{AppID: "control", Branch: "main"}); err == nil {
		t.Fatal("missing principal accepted")
	}
}
