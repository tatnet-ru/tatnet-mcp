package main

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/tatnet-ru/tatnet-mcp/internal/config"
)

const (
	internalSecret = "internal-secret-for-tests"
	liveGrant      = "6f1c2a52-8a3e-4b1d-9d7e-0c5f4e3b2a19"
	publicURL      = "https://mcp.test"
)

// fakeHydra — только JWKS: MCP-сервер сам токены не выдаёт, он их проверяет.
type fakeHydra struct {
	key  *rsa.PrivateKey
	kid  string
	down bool
	srv  *httptest.Server
}

func newFakeHydra(t *testing.T) *fakeHydra {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	h := &fakeHydra{key: k, kid: "k1"}
	h.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.down {
			http.Error(w, "down", http.StatusBadGateway)
			return
		}
		e := big.NewInt(int64(k.E)).Bytes()
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{
			"kty": "RSA", "use": "sig", "kid": h.kid, "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(k.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(e),
		}}})
	}))
	t.Cleanup(h.srv.Close)
	return h
}

func (h *fakeHydra) token(t *testing.T, mut func(jwt.MapClaims)) string {
	t.Helper()
	c := jwt.MapClaims{
		"iss": h.srv.URL, "sub": "user-1", "client_id": "dcr-client",
		"aud": []string{publicURL + "/mcp"},
		"exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(),
		"ext": map[string]any{"tatnet_grant": liveGrant},
	}
	if mut != nil {
		mut(c)
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, c)
	tok.Header["kid"] = h.kid
	s, err := tok.SignedString(h.key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func setupOAuth(t *testing.T) (*fakeAPI, *fakeHydra, *httptest.Server) {
	t.Helper()
	api := newFakeAPI()
	apiSrv := httptest.NewServer(api)
	t.Cleanup(apiSrv.Close)
	hydra := newFakeHydra(t)
	h := Routes(config.Config{
		APIBaseURL: apiSrv.URL, PublicURL: publicURL,
		OIDCIssuer: hydra.srv.URL, OIDCJWKSURL: hydra.srv.URL + "/jwks", InternalSecret: internalSecret,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return api, hydra, srv
}

func initialize(t *testing.T, url, token string) (int, http.Header, string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, url+"/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"x","version":"0"}}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header, string(b)
}

func TestUnauthenticatedClientIsPointedToTheAuthorizationServer(t *testing.T) {
	_, hydra, srv := setupOAuth(t)
	code, hdr, _ := initialize(t, srv.URL, "")
	if code != 401 {
		t.Fatalf("want 401, got %d", code)
	}
	wa := hdr.Get("WWW-Authenticate")
	if !strings.Contains(wa, `resource_metadata="`+publicURL+`/.well-known/oauth-protected-resource/mcp"`) {
		t.Fatalf("WWW-Authenticate must point to resource metadata: %q", wa)
	}
	for _, path := range []string{"/.well-known/oauth-protected-resource/mcp", "/.well-known/oauth-protected-resource"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		var meta map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&meta)
		resp.Body.Close()
		if meta["resource"] != publicURL+"/mcp" {
			t.Errorf("%s: resource = %v", path, meta["resource"])
		}
		if as, _ := meta["authorization_servers"].([]any); len(as) != 1 || as[0] != hydra.srv.URL {
			t.Errorf("%s: authorization_servers = %v", path, meta["authorization_servers"])
		}
	}
}

func TestConnectedAppActsThroughTheGrantChannel(t *testing.T) {
	api, hydra, srv := setupOAuth(t)
	s, err := connect(t, srv.URL, hydra.token(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	out, text, isErr := call(t, s, "whoami", nil)
	if isErr {
		t.Fatal(text)
	}
	if out["account_id"] != "acc1" {
		t.Fatalf("whoami through grant: %v", out)
	}
	if api.grantCalls.Load() == 0 {
		t.Fatal("calls must go through the grant channel")
	}
	// Токен клиента выдан для MCP и дальше него не едет.
	if api.grantLeakedAuth.Load() {
		t.Fatal("the client's token must never be forwarded to /v1")
	}
}

func TestBadTokensAreRejectedWith401(t *testing.T) {
	api, hydra, srv := setupOAuth(t)
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	forged := func() string {
		tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss": hydra.srv.URL, "aud": []string{publicURL + "/mcp"}, "exp": time.Now().Add(time.Hour).Unix(),
			"ext": map[string]any{"tatnet_grant": liveGrant},
		})
		tok.Header["kid"] = hydra.kid
		s, _ := tok.SignedString(other)
		return s
	}()
	cases := map[string]string{
		"no grant claim (not from our consent)": hydra.token(t, func(c jwt.MapClaims) { c["ext"] = map[string]any{} }),
		"grant claim not a uuid":                hydra.token(t, func(c jwt.MapClaims) { c["ext"] = map[string]any{"tatnet_grant": "x"} }),
		"issued for another resource":           hydra.token(t, func(c jwt.MapClaims) { c["aud"] = []string{"https://evil.example/mcp"} }),
		"foreign issuer":                        hydra.token(t, func(c jwt.MapClaims) { c["iss"] = "https://evil.example" }),
		"expired":                               hydra.token(t, func(c jwt.MapClaims) { c["exp"] = time.Now().Add(-2 * time.Hour).Unix() }),
		"no expiry":                             hydra.token(t, func(c jwt.MapClaims) { delete(c, "exp") }),
		"signed by someone else":                forged,
		"garbage":                               "not-a-token",
	}
	for name, tok := range cases {
		if code, _, body := initialize(t, srv.URL, tok); code != 401 {
			t.Errorf("%s: want 401, got %d (%s)", name, code, body)
		}
	}
	// Токен без подключения должен отвергаться САМИМ сервером, а не потому,
	// что /v1 случайно не узнал пустой ключ: иначе вторая линия обороны
	// выдавала бы себя за первую.
	for _, name := range []string{"no grant claim (not from our consent)", "grant claim not a uuid"} {
		if _, _, body := initialize(t, srv.URL, cases[name]); !strings.Contains(body, "consent screen") {
			t.Errorf("%s: must be refused by the grant check, got %q", name, body)
		}
	}
	if n := api.grantCalls.Load(); n != 0 {
		t.Errorf("bad tokens must not reach /v1 via the grant channel (%d calls)", n)
	}
}

func TestRevokedConnectionForcesReconnect(t *testing.T) {
	api, hydra, srv := setupOAuth(t)
	api.grantRevoked.Store(true)
	code, _, body := initialize(t, srv.URL, hydra.token(t, nil))
	if code != 401 || !strings.Contains(body, "revoked") {
		t.Fatalf("revoked connection must be 401 with a reason, got %d %q", code, body)
	}
}

func TestAuthorizationServerOutageIsNotAnInvalidToken(t *testing.T) {
	_, hydra, srv := setupOAuth(t)
	hydra.down = true
	code, _, _ := initialize(t, srv.URL, hydra.token(t, nil))
	// 401 заставил бы клиента выкинуть рабочий токен и гнать человека на вход
	// из-за НАШЕЙ поломки.
	if code == 401 {
		t.Fatal("JWKS outage must not look like an invalid token")
	}
}

func TestAPIKeysStillWorkNextToOAuth(t *testing.T) {
	_, _, srv := setupOAuth(t)
	s, err := connect(t, srv.URL, goodKey)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, text, isErr := call(t, s, "whoami", nil); isErr {
		t.Fatal(text)
	}
}
