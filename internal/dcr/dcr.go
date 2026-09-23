// Package dcr — прослойка регистрации клиентов (RFC 7591) перед Hydra.
//
// Hydra v26 отвечает на регистрацию ВСЕМИ полями клиента, незаданные — пустыми
// строками и null (`client_uri: ""`, `contacts: null`, `logo_uri: ""`…).
// RFC 7591 велит незаданные поля не включать, и строгие клиенты MCP (SDK
// Claude Code, 2026-09-23) такой ответ отвергают: пустая строка — не URL,
// null — не массив. Регистрация проходила, а клиент на ней падал.
//
// Прослойка ничего не решает сама: тело уходит в Hydra как есть, статус
// возвращается как есть, из ответа вырезаются только пустые значения.
// Безопасность регистрации (запрет metadata/skip_consent) остаётся за Hydra.
package dcr

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// Больше этого регистрация не бывает; ограничение — чтобы прослойка не стала
// трубой для чего угодно.
const maxBody = 64 << 10

func Handler(upstream string, hc *http.Client) http.Handler {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Регистрацию делают и браузерные клиенты — CORS открыт, как у
		// метаданных: в ответе ничего чужого, он адресован тому, кто
		// регистрируется.
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "POST")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, maxBody+1))
		if err != nil || len(body) > maxBody {
			writeErr(w, http.StatusBadRequest, "invalid_client_metadata", "registration request is too large or unreadable")
			return
		}
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstream, bytes.NewReader(body))
		if err != nil {
			writeErr(w, http.StatusBadGateway, "server_error", "registration is unavailable")
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		resp, err := hc.Do(req)
		if err != nil {
			writeErr(w, http.StatusBadGateway, "temporarily_unavailable", "authorization server is unavailable, try again later")
			return
		}
		defer resp.Body.Close()
		out, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			writeErr(w, http.StatusBadGateway, "temporarily_unavailable", "authorization server is unavailable, try again later")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(resp.StatusCode)
		_, _ = w.Write(Clean(out))
	})
}

// Clean убирает из JSON-объекта верхнего уровня поля с пустыми значениями:
// "", null, [] и {}. Не объект — возвращается как есть.
func Clean(raw []byte) []byte {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw
	}
	for k, v := range m {
		switch string(bytes.TrimSpace(v)) {
		case `""`, "null", "[]", "{}":
			delete(m, k)
		}
	}
	b, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return b
}

func writeErr(w http.ResponseWriter, code int, errCode, desc string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": errCode, "error_description": desc})
}
