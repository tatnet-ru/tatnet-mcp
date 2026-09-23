// tatnet-mcp — MCP-сервер TatNet: управление платформой из Claude, ChatGPT и
// любого клиента MCP.
//
// Сервер — тонкий слой над публичным /v1: своей БД, своих прав и бизнес-логики
// у него нет. Состояния сессии тоже нет (stateless Streamable HTTP), поэтому
// реплики взаимозаменяемы и выкат не рвёт клиентов.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tatnet-ru/tatnet-mcp/internal/authn"
	"github.com/tatnet-ru/tatnet-mcp/internal/config"
	"github.com/tatnet-ru/tatnet-mcp/internal/metrics"
	"github.com/tatnet-ru/tatnet-mcp/internal/tools"
)

var version = "dev"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	cfg, err := config.Load()
	if err != nil {
		log.Error("не смогли прочитать конфигурацию", "err", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           Routes(cfg, log),
		ReadHeaderTimeout: 15 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()

	log.Info("mcp-сервер запущен", "listen", cfg.Listen, "api", cfg.APIBaseURL, "version", version)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("сервер остановлен с ошибкой", "err", err)
		os.Exit(1)
	}
}

// Routes собирает HTTP-обработчик целиком — отдельно от main, чтобы тест
// гонял ровно то, что обслуживает прод.
func Routes(cfg config.Config, log *slog.Logger) http.Handler {
	t := tools.New(cfg.APIBaseURL)
	server := mcp.NewServer(&mcp.Implementation{Name: "tatnet", Title: "TatNet", Version: version},
		&mcp.ServerOptions{Instructions: tools.Instructions, Logger: log})
	t.Register(server)

	verifier := authn.NewVerifier(cfg.APIBaseURL, &http.Client{Timeout: 10 * time.Second})
	counted := func(ctx context.Context, token string, r *http.Request) (*auth.TokenInfo, error) {
		ti, err := verifier.Verify(ctx, token, r)
		switch {
		case errors.Is(err, auth.ErrInvalidToken):
			metrics.AuthFailures.WithLabelValues("invalid").Inc()
		case err != nil:
			metrics.AuthFailures.WithLabelValues("unavailable").Inc()
		}
		return ti, err
	}
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true, Logger: log})
	// Метаданных защищённого ресурса (RFC 9728) пока нет намеренно: сервера
	// авторизации ещё нет, и пустой список серверов обещал бы вход, которого
	// не существует. Появятся вместе с OAuth.
	protected := auth.RequireBearerToken(counted, &auth.RequireBearerTokenOptions{
		// Ключи /v1 бывают бессрочными — срок токена здесь не обязателен.
		AllowMissingExpiration: true,
	})(mcpHandler)

	mux := http.NewServeMux()
	mux.Handle("/mcp", protected)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.Handle("GET /metrics", metrics.Handler(cfg.MetricsToken))
	return mux
}
