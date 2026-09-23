// tatnet-mcp — MCP-сервер TatNet: управление платформой из Claude, ChatGPT и
// любого клиента MCP.
//
// Сервер — тонкий слой над публичным /v1: своей БД, своих прав и бизнес-логики
// у него нет. Состояния сессии тоже нет (stateless Streamable HTTP), поэтому
// реплики взаимозаменяемы и выкат не рвёт клиентов.
package main

import (
	"context"
	"encoding/json"
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
	server := mcp.NewServer(&mcp.Implementation{Name: "tatnet", Title: "TatNet", Version: version},
		&mcp.ServerOptions{Instructions: tools.Instructions, Logger: log})
	t.Register(server)

	verifier := authn.NewVerifier(authn.Config{
		APIBase: cfg.APIBaseURL, Issuer: cfg.OIDCIssuer, JWKSURL: cfg.OIDCJWKSURL,
		Resource: cfg.Resource(), InternalSecret: cfg.InternalSecret,
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
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server },
		&mcp.StreamableHTTPOptions{Stateless: true, Logger: log})
	// Метаданные защищённого ресурса (RFC 9728): по ссылке из 401 любой клиент
	// MCP находит сервер авторизации, регистрируется там сам (DCR) и ведёт
	// человека на вход и экран согласия. Без OAuth ссылки нет — пустой список
	// серверов обещал бы вход, которого не существует.
	opts := &auth.RequireBearerTokenOptions{
		// Ключи /v1 бывают бессрочными — срок токена здесь не обязателен.
		AllowMissingExpiration: true,
	}
	metadataPath := "/.well-known/oauth-protected-resource/mcp"
	if cfg.OIDCIssuer != "" {
		opts.ResourceMetadataURL = cfg.PublicURL + metadataPath
	}
	protected := auth.RequireBearerToken(counted, opts)(mcpHandler)

	mux := http.NewServeMux()
	mux.Handle("/mcp", protected)
	if cfg.OIDCIssuer != "" {
		meta := map[string]any{
			"resource":                 cfg.Resource(),
			"authorization_servers":    []string{cfg.OIDCIssuer},
			"scopes_supported":         []string{"openid", "offline_access"},
			"bearer_methods_supported": []string{"header"},
			"resource_name":            "TatNet",
		}
		serveMeta := func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Access-Control-Allow-Origin", "*")
			_ = json.NewEncoder(w).Encode(meta)
		}
		// Путь с суффиксом ресурса — по RFC 9728; корневой — для клиентов,
		// которые ищут метаданные у хоста.
		mux.HandleFunc("GET "+metadataPath, serveMeta)
		mux.HandleFunc("GET /.well-known/oauth-protected-resource", serveMeta)
		// Регистрация клиентов через прослойку: Hydra отдаёт пустые поля,
		// которые строгие клиенты отвергают (см. internal/dcr).
		mux.Handle("/oauth/register", dcr.Handler(cfg.OIDCIssuer+"/oauth2/register", nil))
	}
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok\n")) })
	mux.Handle("GET /metrics", metrics.Handler(cfg.MetricsToken))
	return mux
}
