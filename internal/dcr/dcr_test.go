package dcr

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Ответ Hydra v26.2.0 в той форме, которую отверг SDK Claude Code (снято с прода).
const hydraReply = `{"client_id":"c1","client_name":"x","client_uri":"","contacts":null,"logo_uri":"","audience":[],"jwks":{},"metadata":{},
"redirect_uris":["http://127.0.0.1/cb"],"grant_types":["authorization_code","refresh_token"],"token_endpoint_auth_method":"none",
"registration_access_token":"rat","registration_client_uri":"https://auth.example/oauth2/register/c1","client_secret_expires_at":0,"skip_consent":false}`

func TestRegistrationIsForwardedAndEmptyFieldsDropped(t *testing.T) {
	var gotBody string
	hydra := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(hydraReply))
	}))
	defer hydra.Close()
	srv := httptest.NewServer(Handler(hydra.URL, nil, nil))
	defer srv.Close()

	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(`{"client_name":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status must pass through, got %d", resp.StatusCode)
	}
	if gotBody != `{"client_name":"x"}` {
		t.Fatalf("request body must reach Hydra unchanged, got %q", gotBody)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	for _, k := range []string{"client_uri", "contacts", "logo_uri", "audience", "jwks", "metadata"} {
		if _, ok := out[k]; ok {
			t.Errorf("%s must be dropped", k)
		}
	}
	// Значимые поля — в том числе false и 0 — остаются.
	if out["client_id"] != "c1" || out["registration_access_token"] != "rat" || out["skip_consent"] != false || out["client_secret_expires_at"] != float64(0) {
		t.Fatalf("meaningful fields must stay: %v", out)
	}
}

func TestHydraErrorsPassThrough(t *testing.T) {
	hydra := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_client_metadata","error_description":"'metadata' cannot be set"}`))
	}))
	defer hydra.Close()
	srv := httptest.NewServer(Handler(hydra.URL, nil, nil))
	defer srv.Close()
	resp, _ := http.Post(srv.URL, "application/json", strings.NewReader(`{"metadata":{"first_party":true}}`))
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 400 || !strings.Contains(string(b), "cannot be set") {
		t.Fatalf("Hydra's refusal must reach the client: %d %s", resp.StatusCode, b)
	}
}

func TestUnreachableHydraIsNotARegistration(t *testing.T) {
	srv := httptest.NewServer(Handler("http://127.0.0.1:1/unreachable", nil, nil))
	defer srv.Close()
	resp, _ := http.Post(srv.URL, "application/json", strings.NewReader(`{}`))
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("want 502, got %d", resp.StatusCode)
	}
}

func TestOversizedBodyIsRefused(t *testing.T) {
	srv := httptest.NewServer(Handler("http://127.0.0.1:1/never", nil, nil))
	defer srv.Close()
	resp, _ := http.Post(srv.URL, "application/json", strings.NewReader(strings.Repeat("x", maxBody+10)))
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", resp.StatusCode)
	}
}

// Без аудитории в списке клиента Hydra отвергает ОБНОВЛЕНИЕ токена (вход при
// этом проходит): клиент MCP работал час и просил войти заново.
func TestAudienceIsSetOverWhatTheClientSent(t *testing.T) {
	var got map[string]any
	hydra := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(hydraReply))
	}))
	defer hydra.Close()
	aud := []string{"https://mcp.example/mcp", "https://mcp2.example/mcp"}
	srv := httptest.NewServer(Handler(hydra.URL, aud, nil))
	defer srv.Close()

	resp, err := http.Post(srv.URL, "application/json",
		strings.NewReader(`{"client_name":"x","audience":["https://api.example"],"redirect_uris":["http://127.0.0.1/cb"]}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	gotAud, _ := got["audience"].([]any)
	if len(gotAud) != 2 || gotAud[0] != aud[0] || gotAud[1] != aud[1] {
		t.Fatalf("audience must be exactly the server's resources, got %v", got["audience"])
	}
	if got["client_name"] != "x" || got["redirect_uris"] == nil {
		t.Fatalf("other fields must reach Hydra unchanged: %v", got)
	}
}

func TestNonObjectBodyIsForwardedAsIs(t *testing.T) {
	var gotBody string
	hydra := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"invalid_client_metadata"}`))
	}))
	defer hydra.Close()
	srv := httptest.NewServer(Handler(hydra.URL, []string{"https://mcp.example/mcp"}, nil))
	defer srv.Close()

	resp, err := http.Post(srv.URL, "application/json", strings.NewReader(`["not an object"]`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if gotBody != `["not an object"]` || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("non-object must pass through to Hydra's own rejection, got %q %d", gotBody, resp.StatusCode)
	}
}
