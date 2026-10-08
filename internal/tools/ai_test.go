package tools

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type aiTransport func(*http.Request) (*http.Response, error)

func (f aiTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestAICatalogDoesNotForwardCredentials(t *testing.T) {
	tools := New("https://platform.invalid")
	calls := 0
	tools.HTTP.Transport = aiTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.String() != "https://ai.tatnet.cloud/v1/images/models" || len(r.Header) != 0 {
			t.Fatal(r.URL, r.Header)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"object":"list","data":[]}`)), Header: make(http.Header)}, nil
	})
	_, out, err := tools.aiModels(context.Background(), nil, aiModelsInput{Kind: "image"})
	if err != nil || len(out.Catalog) == 0 {
		t.Fatal(out, err)
	}
	if _, _, err := tools.aiModels(context.Background(), nil, aiModelsInput{Kind: "../../../internal"}); err == nil {
		t.Fatal("invalid kind accepted")
	}
	if calls != 1 {
		t.Fatal(calls)
	}
}

func TestAIChatCatalogIsPublic(t *testing.T) {
	for _, kind := range []string{"chat", "text"} {
		t.Run(kind, func(t *testing.T) {
			tools := New("https://platform.invalid")
			tools.HTTP.Transport = aiTransport(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != "https://ai.tatnet.cloud/v1/models" || len(r.Header) != 0 {
					t.Fatal(r.URL, r.Header)
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"data":[{"id":"glm-5.2"}]}`)), Header: make(http.Header)}, nil
			})
			_, out, err := tools.aiModels(context.Background(), nil, aiModelsInput{Kind: kind})
			if err != nil || !strings.Contains(string(out.Catalog), "glm-5.2") {
				t.Fatal(out, err)
			}
		})
	}
}
