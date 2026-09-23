// Package authn проверяет, с чем клиент MCP пришёл, и НЕ решает, что ему можно.
//
// Права решает api: ключ `tn_live_…` уезжает в /v1 как есть, а там его
// политика пересекается с живой ролью создателя на каждом запросе. Здесь
// только отсекаются заведомо негодные токены — чтобы клиент MCP получил 401
// с адресом метаданных сразу при подключении, а не ошибку инструмента на
// первом вызове.
package authn

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/tatnet-ru/tatnet-go/tatnet"
)

// KeyPrefix — формат ключа /v1. Другие токены (OAuth) появятся отдельно.
const KeyPrefix = "tn_live_"

// Сколько помнить УСПЕШНУЮ проверку. Отказы не кешируются никогда: иначе
// только что выпущенный ключ минуту отвечал бы «неверный».
const positiveTTL = 60 * time.Second

// Верхняя граница кеша. Переполнение сбрасывает его целиком — дешевле LRU и
// безопасно: худший исход — лишний запрос whoami.
const maxEntries = 10000

type Principal struct {
	APIKey    string
	AccountID string
	KeyID     string
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

type entry struct {
	p   Principal
	exp time.Time
}

type Verifier struct {
	apiBase string
	hc      *http.Client
	now     func() time.Time

	mu    sync.Mutex
	cache map[[32]byte]entry
}

func NewVerifier(apiBase string, hc *http.Client) *Verifier {
	return &Verifier{apiBase: apiBase, hc: hc, now: time.Now, cache: map[[32]byte]entry{}}
}

// ErrUnavailable — api не ответил. Это не «ключ неверный»: клиенту нельзя
// говорить «перелогиньтесь», когда сломались мы.
var ErrUnavailable = errors.New("TatNet API is unavailable, try again later")

// Verify — TokenVerifier для auth.RequireBearerToken.
func (v *Verifier) Verify(ctx context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
	if !strings.HasPrefix(token, KeyPrefix) {
		return nil, fmt.Errorf("%w: expected a TatNet API key (%s…)", auth.ErrInvalidToken, KeyPrefix)
	}
	h := sha256.Sum256([]byte(token))

	v.mu.Lock()
	e, ok := v.cache[h]
	v.mu.Unlock()
	if ok && v.now().Before(e.exp) {
		return info(e.p), nil
	}

	c, err := tatnet.NewClientWithResponses(v.apiBase, tatnet.WithAPIKey(token), tatnet.WithHTTPClient(v.hc))
	if err != nil {
		return nil, err
	}
	resp, err := c.AccountWhoamiWithResponse(ctx)
	if err != nil {
		return nil, ErrUnavailable
	}
	switch {
	case resp.StatusCode() == http.StatusOK && resp.JSON200 != nil:
	case resp.StatusCode() == http.StatusUnauthorized:
		return nil, fmt.Errorf("%w: the API key is invalid, revoked or expired", auth.ErrInvalidToken)
	default:
		return nil, ErrUnavailable
	}

	p := Principal{APIKey: token, AccountID: resp.JSON200.AccountId, KeyID: resp.JSON200.KeyId}
	v.mu.Lock()
	if len(v.cache) >= maxEntries {
		v.cache = map[[32]byte]entry{}
	}
	v.cache[h] = entry{p: p, exp: v.now().Add(positiveTTL)}
	v.mu.Unlock()
	return info(p), nil
}

func info(p Principal) *auth.TokenInfo {
	return &auth.TokenInfo{
		// UserID привязывает сессию к ключу: чужой токен не продолжит её.
		UserID: p.KeyID,
		Extra:  map[string]any{"principal": p},
	}
}
