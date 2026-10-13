package tools

import (
	"context"
	"net/http"
	"testing"
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
func TestKafkaPathEscapesTenantIdentifiers(t *testing.T) {
	if got := kafkaPath("p/other", "k/other"); got != "/projects/p%2Fother/kafka-clusters/k%2Fother" {
		t.Fatal(got)
	}
}
