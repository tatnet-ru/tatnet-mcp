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

type kafkaRoundTrip func(*http.Request) (*http.Response, error)

func (f kafkaRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestKafkaRequestRequiresPrincipalBeforeNetwork(t *testing.T) {
	tool := New("https://api.invalid/v1")
	tool.HTTP = &http.Client{Transport: kafkaRoundTrip(func(*http.Request) (*http.Response, error) {
		t.Fatal("unauthenticated request escaped to API")
		return nil, nil
	})}
	_, _, err := tool.kafkaRequest(context.Background(), "GET", "/kafka/regions", nil)
	if err == nil {
		t.Fatal("missing principal accepted")
	}
}

func TestKafkaResumeForwardsPrincipalAndReturnsHourlyPrice(t *testing.T) {
	tool := New("https://api.invalid/v1")
	tool.HTTP.Transport = kafkaRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.Method != "POST" || r.URL.Path != "/v1/projects/p/kafka-clusters/k/resume" || r.Header.Get("Authorization") != "Bearer tn_live_test" {
			t.Fatalf("wrong resume request: %s %s", r.Method, r.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"id":"k","hourly_cost":3.44773}`))}, nil
	})
	handler := auth.RequireBearerToken(func(context.Context, string, *http.Request) (*auth.TokenInfo, error) {
		return &auth.TokenInfo{Expiration: time.Now().Add(time.Minute), Extra: map[string]any{"principal": authn.Principal{APIKey: "tn_live_test"}}}, nil
	}, nil)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, out, err := tool.kafkaResume(r.Context(), nil, KafkaClusterIn{ProjectID: "p", ClusterID: "k"})
		if err != nil || out.Data["hourly_cost"] != 3.44773 {
			t.Fatalf("resume response: %v %v", out, err)
		}
	}))
	req := httptest.NewRequest("POST", "/mcp", nil)
	req.Header.Set("Authorization", "Bearer tn_live_test")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	if response.Code != 200 {
		t.Fatalf("authentication rejected: %d", response.Code)
	}
}
func TestKafkaPathEscapesTenantIdentifiers(t *testing.T) {
	if got := kafkaPath("p/other", "k/other"); got != "/projects/p%2Fother/kafka-clusters/k%2Fother" {
		t.Fatal(got)
	}
}
