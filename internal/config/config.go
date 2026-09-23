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
	// PublicURL — ОСНОВНОЙ внешний адрес сервера (https://mcp.tatnet.cloud).
	// Нужен для метаданных защищённого ресурса (RFC 9728), по которым клиент
	// MCP находит, где брать токен.
	PublicURL string
	// ExtraPublicURLs — дополнительные имена, на которых сервер тоже отвечает
	// (переходное https://mcp.tatnet.ru). На каждом имени сервер говорит от
	// его лица: токен клиента привязан к адресу, по которому тот подключался.
	ExtraPublicURLs []string
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

// Resource — адрес MCP-ресурса основного имени (RFC 8707/9728).
func (c Config) Resource() string { return c.PublicURL + "/mcp" }

// PublicURLs — все имена сервера, основное первым.
func (c Config) PublicURLs() []string { return append([]string{c.PublicURL}, c.ExtraPublicURLs...) }

func Load() (Config, error) {
	c := Config{
		Listen:         listenAddr(),
		APIBaseURL:     strings.TrimRight(env("API_BASE_URL", "https://api.tatnet.ru/v1"), "/"),
		PublicURL:      strings.TrimRight(env("PUBLIC_URL", "https://mcp.tatnet.cloud"), "/"),
		MetricsToken:   os.Getenv("METRICS_TOKEN"),
		OIDCIssuer:     strings.TrimRight(os.Getenv("OIDC_ISSUER"), "/"),
		InternalSecret: strings.TrimSpace(os.Getenv("MCP_INTERNAL_SECRET")),
	}
	for _, u := range strings.Split(os.Getenv("EXTRA_PUBLIC_URLS"), ",") {
		if u = strings.TrimRight(strings.TrimSpace(u), "/"); u != "" {
			c.ExtraPublicURLs = append(c.ExtraPublicURLs, u)
		}
	}
	c.OIDCJWKSURL = env("OIDC_JWKS_URL", c.OIDCIssuer+"/.well-known/jwks.json")
	if c.OIDCIssuer != "" && c.InternalSecret == "" {
		// Токен клиента в /v1 не пробрасывается, а без секрета ходить туда
		// «от имени подключения» нечем: вход бы работал, а каждый вызов — нет.
		return c, fmt.Errorf("OIDC_ISSUER задан, а MCP_INTERNAL_SECRET нет")
	}
	for _, v := range append([]string{c.APIBaseURL}, c.PublicURLs()...) {
		u, err := url.Parse(v)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return c, fmt.Errorf("не абсолютный URL: %q", v)
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

// listenAddr: явный LISTEN (хост, стенд) → PORT, который отдаёт Apps Platform
// → :8080. Без PORT сервер слушал бы не тот порт, на который платформа шлёт
// трафик: живой процесс без единого ответа.
func listenAddr() string {
	if v := strings.TrimSpace(os.Getenv("LISTEN")); v != "" {
		return v
	}
	if p := strings.TrimSpace(os.Getenv("PORT")); p != "" {
		return ":" + p
	}
	return ":8080"
}
