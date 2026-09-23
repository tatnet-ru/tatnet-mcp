package authn

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// jwks — ключи подписи Hydra. Перечитываются, когда приходит токен с
// незнакомым kid (Hydra ротировала ключ), но не чаще раза в refetchGap: иначе
// поток мусорных токенов с выдуманными kid долбил бы сервер авторизации.
type jwks struct {
	url string
	hc  *http.Client
	now func() time.Time

	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

const refetchGap = 30 * time.Second

var errUnknownKey = errors.New("unknown signing key")

func newJWKS(url string, hc *http.Client) *jwks {
	return &jwks{url: url, hc: hc, now: time.Now, keys: map[string]*rsa.PublicKey{}}
}

func (j *jwks) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if k, ok := j.keys[kid]; ok {
		return k, nil
	}
	if !j.fetchedAt.IsZero() && j.now().Sub(j.fetchedAt) < refetchGap {
		return nil, errUnknownKey
	}
	if err := j.fetch(ctx); err != nil {
		return nil, err
	}
	if k, ok := j.keys[kid]; ok {
		return k, nil
	}
	return nil, errUnknownKey
}

func (j *jwks) fetch(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.url, nil)
	if err != nil {
		return err
	}
	resp, err := j.hc.Do(req)
	if err != nil {
		return fmt.Errorf("jwks: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks: HTTP %d", resp.StatusCode)
	}
	var doc struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			Use string `json:"use"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("jwks: %w", err)
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range doc.Keys {
		if k.Kty != "RSA" || (k.Use != "" && k.Use != "sig") {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil || len(e) > 4 {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(new(big.Int).SetBytes(e).Int64())}
	}
	j.keys = keys
	j.fetchedAt = j.now()
	return nil
}
