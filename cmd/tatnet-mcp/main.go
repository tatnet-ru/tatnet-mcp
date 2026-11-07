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
	"strings"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/tatnet-ru/tatnet-mcp/internal/authn"
	"github.com/tatnet-ru/tatnet-mcp/internal/config"
	"github.com/tatnet-ru/tatnet-mcp/internal/dcr"
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
	t.InternalSecret = cfg.InternalSecret

	verifier := authn.NewVerifier(authn.Config{
		APIBase: cfg.APIBaseURL, Issuer: cfg.OIDCIssuer, JWKSURL: cfg.OIDCJWKSURL,
		Resources: resources(cfg), InternalSecret: cfg.InternalSecret,
	}, &http.Client{Timeout: 10 * time.Second})
	counted := func(ctx context.Context, token string, r *http.Request) (*auth.TokenInfo, error) {
		ti, err := verifier.Verify(ctx, token, r)
		kind := "jwt"
		if strings.HasPrefix(token, authn.KeyPrefix) {
			kind = "api_key"
		}
		// Причина отказа — в лог: без неё «клиент не подключился» не
		// разобрать. Сам токен не пишется никогда.
		switch {
		case errors.Is(err, auth.ErrInvalidToken):
			metrics.AuthFailures.WithLabelValues("invalid").Inc()
			log.Warn("токен отвергнут", "kind", kind, "reason", err.Error(), "ua", r.UserAgent())
		case err != nil:
			metrics.AuthFailures.WithLabelValues("unavailable").Inc()
			log.Error("проверка токена не удалась", "kind", kind, "reason", err.Error())
		}
		return ti, err
	}
	mux := http.NewServeMux()
	for _, resource := range mcpResources {
		server := mcp.NewServer(&mcp.Implementation{Name: resource.implementation, Title: resource.name, Version: version},
			&mcp.ServerOptions{Instructions: resource.instructions, Logger: log})
		resource.register(t, server)
		mountMCPResource(mux, cfg, log, resource, server, counted)
	}
	if cfg.OIDCIssuer != "" {
		var audiences []string
		for _, pub := range cfg.PublicURLs() {
			for _, resource := range mcpResources {
				audiences = append(audiences, pub+resource.path)
			}
		}
		mux.Handle("/oauth/register", dcr.Handler(cfg.OIDCIssuer+"/oauth2/register", audiences, nil))
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.Handle("GET /metrics", metrics.Handler(cfg.MetricsToken))
	return mux
}
