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
	"github.com/tatnet-ru/tatnet-go/tatnet"
	"github.com/tatnet-ru/tatnet-mcp/internal/authn"
)

func TestCreateVMKeepsBackupsInSingleAuthenticatedRequest(t *testing.T) {
	tool := New("https://api.invalid/v1")
	calls := 0
	tool.HTTP.Transport = kafkaRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "POST" || r.URL.Path != "/v1/projects/p1/vms" || r.Header.Get("Authorization") != "Bearer tn_live_test" {
			t.Fatalf("wrong request %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		policy := body["backup_policy"].(map[string]any)
		if body["name"] != "backup-vm" || policy["enabled"] != true || policy["retention_days"] != float64(14) {
			t.Fatalf("lost VM or policy: %v", body)
		}
		return &http.Response{StatusCode: 201, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"vm1","status":"pending"}`))}, nil
	})
	handler := auth.RequireBearerToken(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return &auth.TokenInfo{Expiration: time.Now().Add(time.Minute), Extra: map[string]any{"principal": authn.Principal{APIKey: "tn_live_test"}}}, nil
	}, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enabled := true
		hour, days := 6, 14
		_, out, err := tool.createVM(r.Context(), nil, CreateVMIn{ProjectID: "p1", VM: tatnet.V1VMCreate{Name: "backup-vm", BackupPolicy: &tatnet.VMBackupPolicyRequest{Enabled: &enabled, HourUtc: &hour, RetentionDays: &days}}})
		if err != nil || out.Data["id"] != "vm1" {
			t.Fatalf("result %v %v", out, err)
		}
	}))
	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer tn_live_test")
	handler.ServeHTTP(httptest.NewRecorder(), req)
	if calls != 1 {
		t.Fatalf("sent %d requests", calls)
	}
	if _, _, err := tool.createVM(context.Background(), nil, CreateVMIn{}); err == nil {
		t.Fatal("missing principal accepted")
	}
	if calls != 1 {
		t.Fatal("unauthenticated request escaped")
	}
}
