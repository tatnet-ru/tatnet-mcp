// Package authn проверяет, с чем клиент MCP пришёл, и НЕ решает, что ему можно.
//
// Токены двух видов:
//
//   - ключ /v1 `tn_live_…` (Claude Code, CI, свои скрипты) — уезжает в /v1 как
//     есть;
//   - токен Hydra (`auth.tatnet.ru`), который клиент получил сам: регистрация
//     (DCR), вход, экран согласия. В нём поле `tatnet_grant` — id подключения,
//     выпущенного на согласии. Этот токен выдан ДЛЯ MCP и дальше него не едет:
//     спецификация MCP запрещает пробрасывать его в чужой API. В /v1 сервер
//     ходит сам, служебным каналом «от имени подключения».
//
// Права в обоих случаях решает api: политика ключа или подключения
// пересекается с живой ролью человека на каждом запросе.
package authn

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/tatnet-ru/tatnet-go/tatnet"
)

// KeyPrefix — формат ключа /v1.
const KeyPrefix = "tn_live_"

// Заголовки служебного канала — пара к api `v1_auth.GRANT_HEADER`/`MCP_SECRET_HEADER`.
const (
	GrantHeader  = "X-TatNet-Grant"
	SecretHeader = "X-TatNet-Internal-Secret"
	GrantClaim   = "tatnet_grant"
)

// Сколько помнить УСПЕШНУЮ проверку. Отказы не кешируются никогда: иначе
// только что выпущенный ключ минуту отвечал бы «неверный». Этот же срок —
// предел, за который отзыв подключения в панели доходит до клиента.
const positiveTTL = 60 * time.Second

const maxEntries = 10000

// Principal — от чьего имени идут вызовы /v1. Ровно одно из APIKey и Grant.
type Principal struct {
	APIKey    string
	Grant     string
	AccountID string
	KeyID     string
}

// Editor — как подписать запрос к /v1 от имени этого принципала.
func (p Principal) Editor(internalSecret string) tatnet.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error {
		if p.Grant != "" {
			req.Header.Set(GrantHeader, p.Grant)
			req.Header.Set(SecretHeader, internalSecret)
			return nil
		}
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
		return nil
	}
}

// FromContext достаёт принципала, положенного верификатором.
func FromContext(ctx context.Context) (Principal, bool) {
	ti := auth.TokenInfoFromContext(ctx)
	if ti == nil {
		return Principal{}, false
	}
	p, ok := ti.Extra["principal"].(Principal)
	return p, ok
}

type Config struct {
	APIBase string
	// Issuer и JWKSURL сервера авторизации (Hydra). Пустой Issuer — вход через
	// OAuth выключен, принимаются только ключи.
	Issuer  string
	JWKSURL string
	// Resource — адрес этого сервера (`https://mcp.tatnet.ru/mcp`): токен обязан
	// быть выдан для него.
	Resource string
	// InternalSecret — служебный канал к /v1 для подключений.
	InternalSecret string
}

type entry struct {
	p   Principal
	exp time.Time
}

type Verifier struct {
	cfg  Config
	hc   *http.Client
	keys *jwks
	now  func() time.Time

	mu    sync.Mutex
	cache map[[32]byte]entry
}

func NewVerifier(cfg Config, hc *http.Client) *Verifier {
	v := &Verifier{cfg: cfg, hc: hc, now: time.Now, cache: map[[32]byte]entry{}}
	if cfg.Issuer != "" {
		v.keys = newJWKS(cfg.JWKSURL, hc)
	}
	return v
}

// ErrUnavailable — api не ответил. Это не «токен неверный»: клиенту нельзя
// говорить «войдите заново», когда сломались мы.
var ErrUnavailable = errors.New("TatNet API is unavailable, try again later")

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{auth.ErrInvalidToken}, a...)...)
}

// Verify — TokenVerifier для auth.RequireBearerToken.
func (v *Verifier) Verify(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
	h := sha256.Sum256([]byte(token))
	v.mu.Lock()
	e, ok := v.cache[h]
	v.mu.Unlock()
	if ok && v.now().Before(e.exp) {
		return info(e.p, e.exp), nil
	}

	var p Principal
	var expires time.Time
	switch {
	case strings.HasPrefix(token, KeyPrefix):
		p = Principal{APIKey: token}
	case v.keys != nil && strings.Count(token, ".") == 2:
		grant, exp, err := v.parseJWT(ctx, token)
		if err != nil {
			return nil, err
		}
		p, expires = Principal{Grant: grant}, exp
	default:
		return nil, invalid("expected a TatNet API key (%s…) or a token from %s", KeyPrefix, orNone(v.cfg.Issuer))
	}

	// Живо ли то, от чьего имени пришли: ключ не отозван, подключение не
	// отключено в панели, человек всё ещё в аккаунте. Решает api — тем же
	// путём, каким потом пойдут вызовы.
	c, err := tatnet.NewClientWithResponses(v.cfg.APIBase, tatnet.WithHTTPClient(v.hc),
		tatnet.WithRequestEditorFn(p.Editor(v.cfg.InternalSecret)))
	if err != nil {
		return nil, err
	}
	resp, err := c.AccountWhoamiWithResponse(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	switch {
	case resp.StatusCode() == http.StatusOK && resp.JSON200 != nil:
	case resp.StatusCode() == http.StatusUnauthorized && p.Grant != "":
		return nil, invalid("this connection was revoked in the TatNet dashboard; connect again")
	case resp.StatusCode() == http.StatusUnauthorized:
		return nil, invalid("the API key is invalid, revoked or expired")
	default:
		return nil, ErrUnavailable
	}
	p.AccountID, p.KeyID = resp.JSON200.AccountId, resp.JSON200.KeyId

	cacheUntil := v.now().Add(positiveTTL)
	if !expires.IsZero() && expires.Before(cacheUntil) {
		cacheUntil = expires
	}
	v.mu.Lock()
	if len(v.cache) >= maxEntries {
		v.cache = map[[32]byte]entry{}
	}
	v.cache[h] = entry{p: p, exp: cacheUntil}
	v.mu.Unlock()
	return info(p, expires), nil
}

// parseJWT проверяет токен Hydra и достаёт из него id подключения.
func (v *Verifier) parseJWT(ctx context.Context, token string) (string, time.Time, error) {
	var claims struct {
		jwt.RegisteredClaims
		Ext map[string]any `json:"ext"`
	}
	// Отказ получить ключи — наша сторона, а не плохой токен: библиотека
	// заворачивает любую ошибку keyfunc в «не проверяем», поэтому причина
	// запоминается здесь, иначе недоступная Hydra выглядела бы для клиента как
	// «войдите заново».
	var fetchErr error
	_, err := jwt.ParseWithClaims(token, &claims, func(t *jwt.Token) (any, error) {
		kid, _ := t.Header["kid"].(string)
		k, err := v.keys.key(ctx, kid)
		if err != nil && !errors.Is(err, errUnknownKey) {
			fetchErr = err
		}
		return k, err
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(v.cfg.Issuer),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(60*time.Second),
	)
	if fetchErr != nil {
		return "", time.Time{}, ErrUnavailable
	}
	if err != nil {
		return "", time.Time{}, invalid("%v", err)
	}
	// Аудиторию ставит api на согласии; свою `audience` клиент при
	// регистрации может прописать какую угодно, поэтому одна она ничего не
	// доказывает — но токен, выданный для ДРУГОГО ресурса, здесь не годится.
	if !slices.Contains(claims.Audience, v.cfg.Resource) {
		return "", time.Time{}, invalid("token was not issued for %s", v.cfg.Resource)
	}
	// Доверие держится на этом поле: его кладёт только экран согласия TatNet.
	grant, _ := claims.Ext[GrantClaim].(string)
	if _, err := uuid.Parse(grant); err != nil {
		return "", time.Time{}, invalid("token carries no TatNet connection; connect through the TatNet consent screen")
	}
	return grant, claims.ExpiresAt.Time, nil
}

func info(p Principal, exp time.Time) *auth.TokenInfo {
	uid := p.KeyID
	if p.Grant != "" {
		uid = "grant:" + p.Grant
	}
	return &auth.TokenInfo{
		// UserID привязывает сессию к ключу или подключению.
		UserID:     uid,
		Expiration: exp,
		Extra:      map[string]any{"principal": p},
	}
}

func orNone(s string) string {
	if s == "" {
		return "(OAuth is not configured)"
	}
	return s
}
