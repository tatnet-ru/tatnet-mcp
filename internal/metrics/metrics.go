// Package metrics — счётчики MCP-сервера.
package metrics

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	ToolCalls = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tatnet_mcp_tool_calls_total",
		Help: "Вызовы инструментов MCP по исходу (ok | error).",
	}, []string{"tool", "outcome"})

	ToolDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "tatnet_mcp_tool_duration_seconds",
		Help:    "Длительность вызова инструмента, включая ожидание сборки.",
		Buckets: []float64{0.1, 0.25, 0.5, 1, 2.5, 5, 10, 20, 30, 45, 60},
	}, []string{"tool"})

	AuthFailures = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tatnet_mcp_auth_failures_total",
		Help: "Отказы при проверке токена: invalid (плохой токен) | unavailable (api не ответил).",
	}, []string{"reason"})
)

// Handler закрывает /metrics токеном: апп торчит в интернет, метрики — нет.
func Handler(token string) http.Handler {
	h := promhttp.Handler()
	want := []byte(strings.TrimSpace(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(want) == 0 {
			http.Error(w, "metrics disabled: METRICS_TOKEN is not set", http.StatusNotFound)
			return
		}
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok || subtle.ConstantTimeCompare([]byte(strings.TrimSpace(got)), want) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="metrics"`)
			http.Error(w, "bearer token required", http.StatusUnauthorized)
			return
		}
		h.ServeHTTP(w, r)
	})
}
