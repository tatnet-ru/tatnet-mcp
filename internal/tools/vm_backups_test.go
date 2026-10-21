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

func TestVMBackupPolicyUsesAuthenticatedPublicAPI(t *testing.T) {
	tool := New("https://api.invalid/v1")
	tool.HTTP.Transport = kafkaRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method != "PUT" || r.URL.Path != "/v1/projects/p1/vms/vm1/backup-policy" || r.Header.Get("Authorization") != "Bearer tn_live_test" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body) != 3 || body["enabled"] != true || body["retention_days"] != float64(14) {
			t.Fatalf("unexpected body %v", body)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"vpc-id","name":"New network"}`))}, nil
	})
	handler := auth.RequireBearerToken(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return &auth.TokenInfo{Expiration: time.Now().Add(time.Minute), Extra: map[string]any{"principal": authn.Principal{APIKey: "tn_live_test"}}}, nil
	}, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, out, err := tool.setVMBackupPolicy(r.Context(), nil, SetVMBackupPolicyIn{ProjectID: "p1", VMID: "vm1", Enabled: true, HourUTC: 3, RetentionDays: 14})
		if err != nil || out.Data["name"] != "New network" {
			t.Fatalf("result %v %v", out, err)
		}
	}))
	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer tn_live_test")
	handler.ServeHTTP(httptest.NewRecorder(), req)
}
func TestVMBackupPolicyRequiresPrincipal(t *testing.T) {
	tool := New("https://api.invalid/v1")
	tool.HTTP.Transport = kafkaRoundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("unauthenticated request escaped")
		return nil, nil
	})
	if _, _, err := tool.setVMBackupPolicy(context.Background(), nil, SetVMBackupPolicyIn{}); err == nil {
		t.Fatal("missing principal accepted")
	}
}

func TestDeleteVMBackupUsesAuthenticatedAPIAndPreservesConflict(t *testing.T) {
	tool := New("https://api.invalid/v1")
	calls := 0
	tool.HTTP.Transport = kafkaRoundTrip(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Method != "DELETE" || r.URL.Path != "/v1/projects/p1/vms/vm1/backups/old" || r.Header.Get("Authorization") != "Bearer tn_live_test" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		return &http.Response{StatusCode: 409, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"detail":"Backup capture is still in progress"}`))}, nil
	})
	handler := auth.RequireBearerToken(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return &auth.TokenInfo{Expiration: time.Now().Add(time.Minute), Extra: map[string]any{"principal": authn.Principal{APIKey: "tn_live_test"}}}, nil
	}, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result, _, err := tool.deleteVMBackup(r.Context(), nil, DeleteVMBackupIn{ProjectID: "p1", VMID: "vm1", BackupID: "old"})
		if err == nil && (result == nil || !result.IsError) {
			t.Fatal("API rejection was hidden")
		}
	}))
	request := httptest.NewRequest("POST", "/mcp", nil)
	request.Header.Set("Authorization", "Bearer tn_live_test")
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if _, _, err := tool.deleteVMBackup(context.Background(), nil, DeleteVMBackupIn{}); err == nil {
		t.Fatal("unauthenticated deletion accepted")
	}
	if calls != 1 {
		t.Fatalf("unexpected request count %d", calls)
	}
}
