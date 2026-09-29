package main

import (
	"encoding/json"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
)

// server.json — карточка сервера в официальном реестре MCP. Реестр проверяет
// её только при публикации, то есть руками и поздно. Здесь — его правила,
// которые легко нарушить правкой (описание 1..100 символов, транспорт
// streamable-http, версия), и одно НАШЕ: адрес сервера лежит на домене
// пространства имён. Реестр этого не требует (сверено с его валидатором
// 29.09.2026), но пространство имён подтверждается TXT-записью в зоне домена,
// и адрес на чужом домене сделал бы это подтверждение пустым для читателя
// карточки.
func TestServerJSONFitsTheRegistry(t *testing.T) {
	raw, err := os.ReadFile("../../server.json")
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Version     string `json:"version"`
		Remotes     []struct {
			Type string `json:"type"`
			URL  string `json:"url"`
		} `json:"remotes"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	if n := len([]rune(s.Description)); n == 0 || n > 100 {
		t.Errorf("description: %d chars, registry allows 1..100", n)
	}
	if s.Version == "" {
		t.Error("version is required")
	}
	ns, _, ok := strings.Cut(s.Name, "/")
	if !ok {
		t.Fatalf("name %q: want <reverse-domain>/<name>", s.Name)
	}
	labels := strings.Split(ns, ".")
	slices.Reverse(labels)
	domain := strings.Join(labels, ".")
	if len(s.Remotes) == 0 {
		t.Fatal("no remotes")
	}
	for _, r := range s.Remotes {
		if r.Type != "streamable-http" {
			t.Errorf("remote %s: type %q, want streamable-http", r.URL, r.Type)
		}
		u, err := url.Parse(r.URL)
		if err != nil || u.Scheme != "https" {
			t.Errorf("remote %q: want an https URL", r.URL)
			continue
		}
		if h := u.Hostname(); h != domain && !strings.HasSuffix(h, "."+domain) {
			t.Errorf("remote host %s is outside namespace %s (%s)", h, ns, domain)
		}
	}
}
