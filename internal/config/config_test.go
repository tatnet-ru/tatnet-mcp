package config

import "testing"

func TestListenFollowsPlatformPort(t *testing.T) {
	cases := []struct{ listen, port, want string }{
		{"", "", ":8080"},
		{"", "9123", ":9123"},
		{"127.0.0.1:7000", "9123", "127.0.0.1:7000"},
	}
	for _, c := range cases {
		t.Setenv("LISTEN", c.listen)
		t.Setenv("PORT", c.port)
		cfg, err := Load()
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Listen != c.want {
			t.Errorf("LISTEN=%q PORT=%q → %q, want %q", c.listen, c.port, cfg.Listen, c.want)
		}
	}
}

func TestOAuthWithoutInternalSecretRefusesToStart(t *testing.T) {
	t.Setenv("OIDC_ISSUER", "https://auth.example")
	t.Setenv("MCP_INTERNAL_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("OAuth without the internal secret must fail at start, not on every call")
	}
}
