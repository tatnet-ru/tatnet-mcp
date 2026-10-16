package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tatnet-ru/tatnet-mcp/internal/config"
)

func TestS3RegionalTools(t *testing.T) {
	fake := newFakeAPI()
	calls := 0
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/object-storage/regions" && r.URL.Path != "/object-storage/connection-info" && r.URL.Path != "/object-storage/buckets" {
			fake.ServeHTTP(w, r)
			return
		}
		calls++
		if r.Header.Get("Authorization") != "Bearer "+goodKey {
			t.Error("missing user auth")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/object-storage/regions":
			w.Write([]byte(`{"regions":[{"region":"spb-1","endpoint":"https://spb-1.s3.example","enabled":true}]}`))
		case "/object-storage/connection-info":
			if r.URL.Query().Get("region") != "spb-1" {
				t.Error("missing region query")
			}
			w.Write([]byte(`{"region":"spb-1","endpoint":"https://spb-1.s3.example","path_style_example":"x","virtual_hosted_example":"y"}`))
		default:
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["region"] != "spb-1" || body["name"] != "photos" {
				t.Errorf("body=%v", body)
			}
			w.WriteHeader(201)
			w.Write([]byte(`{"id":"one","name":"photos","region":"spb-1","status":"active","is_public":false}`))
		}
	}))
	defer api.Close()
	srv := httptest.NewServer(Routes(config.Config{APIBaseURL: api.URL, PublicURL: "https://mcp.test"}, slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer srv.Close()
	session, err := connect(t, srv.URL, goodKey)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for _, test := range []struct {
		name string
		args map[string]any
	}{
		{"list_s3_regions", map[string]any{}},
		{"get_s3_connection_info", map[string]any{"region": "spb-1"}},
		{"create_s3_bucket", map[string]any{"region": "spb-1", "name": "photos"}},
	} {
		out, text, bad := call(t, session, test.name, test.args)
		if bad {
			t.Fatalf("%s: %s", test.name, text)
		}
		if test.name != "list_s3_regions" && out["region"] != "spb-1" {
			t.Errorf("out=%v", out)
		}
	}
	if calls != 3 {
		t.Fatalf("calls=%d", calls)
	}
}
