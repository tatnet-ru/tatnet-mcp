// Package config читает окружение MCP-сервера.
package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	// Listen — адрес HTTP-сервера. Апп платформы слушает то, что ему дали.
	Listen string
	// APIBaseURL — публичный /v1. MCP ходит туда ровно как CLI и SDK: своей
	// бизнес-логики, своей БД и своих прав у него нет.
	APIBaseURL string
	// PublicURL — внешний адрес самого сервера (https://mcp.tatnet.ru). Нужен
	// для метаданных защищённого ресурса (RFC 9728), по которым клиент MCP
	// находит, где брать токен.
	PublicURL string
	// MetricsToken закрывает /metrics: апп торчит в интернет, а метрики — нет.
	MetricsToken string
	// OIDCIssuer — сервер авторизации (Hydra), выдающий токены клиентам MCP.
	// Пусто — вход через OAuth выключен, работают только ключи /v1.
	OIDCIssuer string
	// OIDCJWKSURL — ключи подписи Hydra. Отдельно от издателя: изнутри
	// площадки публичный адрес может быть недоступен (хайрпин), и ключи тогда
	// берутся с внутреннего.
	OIDCJWKSURL string
	// InternalSecret — служебный канал к /v1 «от имени подключения»; пара к
	// V1_MCP_INTERNAL_SECRET у api. Без него OAuth не включается.
	InternalSecret string
}

// Resource — адрес MCP-ресурса, для которого выдаются токены (RFC 8707/9728).
func (c Config) Resource() string { return c.PublicURL + "/mcp" }

func Load() (Config, error) {
	c := Config{
		Listen:         env("LISTEN", ":8080"),
		APIBaseURL:     strings.TrimRight(env("API_BASE_URL", "https://api.tatnet.ru/v1"), "/"),
		PublicURL:      strings.TrimRight(env("PUBLIC_URL", "https://mcp.tatnet.ru"), "/"),
		MetricsToken:   os.Getenv("METRICS_TOKEN"),
		OIDCIssuer:     strings.TrimRight(os.Getenv("OIDC_ISSUER"), "/"),
		InternalSecret: strings.TrimSpace(os.Getenv("MCP_INTERNAL_SECRET")),
	}
	c.OIDCJWKSURL = env("OIDC_JWKS_URL", c.OIDCIssuer+"/.well-known/jwks.json")
	if c.OIDCIssuer != "" && c.InternalSecret == "" {
		// Токен клиента в /v1 не пробрасывается, а без секрета ходить туда
		// «от имени подключения» нечем: вход бы работал, а каждый вызов — нет.
		return c, fmt.Errorf("OIDC_ISSUER задан, а MCP_INTERNAL_SECRET нет")
	}
	for name, v := range map[string]string{"API_BASE_URL": c.APIBaseURL, "PUBLIC_URL": c.PublicURL} {
		u, err := url.Parse(v)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return c, fmt.Errorf("%s: не абсолютный URL: %q", name, v)
		}
	}
	return c, nil
}

func env(name, def string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return def
}
