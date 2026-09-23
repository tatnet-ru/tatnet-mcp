package tools

import (
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
)

// apiError превращает не-2xx ответ /v1 в текст, по которому модель может
// ДЕЙСТВОВАТЬ: что случилось и что делать дальше. Голое «HTTP 403» модель
// пересказала бы пользователю как «что-то сломалось».
func apiError(op string, resp *http.Response, body []byte) error {
	status := 0
	if resp != nil {
		status = resp.StatusCode
	}
	msg := serverMessage(body)
	var hint string
	switch {
	case status == http.StatusUnauthorized:
		hint = "The API key is invalid, revoked or expired. The user has to create a new key in the TatNet dashboard (API keys) and reconnect."
	case status == http.StatusForbidden:
		hint = "The API key's policy does not allow this action. The user can widen the key's permissions in the TatNet dashboard (API keys); do not retry."
	case status == http.StatusNotFound:
		hint = "Not found — or not visible to this API key (keys can be limited to some projects). Check the id with list_apps / whoami."
	case status == http.StatusConflict:
		hint = "Conflicts with the current state; read the resource again before retrying."
	case status == http.StatusRequestEntityTooLarge:
		hint = "Too large; deploy big projects from a git repository instead."
	case status == http.StatusTooManyRequests:
		ra := ""
		if resp != nil {
			ra = resp.Header.Get("Retry-After")
		}
		hint = "Rate limited; retry after " + orDefault(ra, "a few") + " seconds."
	case status == http.StatusUnprocessableEntity || status == http.StatusBadRequest:
		hint = "The request was rejected as invalid; fix the arguments according to the message."
	case status >= 500:
		hint = "TatNet API error on our side; retry later. This is not caused by the arguments."
	}
	parts := []string{fmt.Sprintf("%s failed (HTTP %d)", op, status)}
	if msg != "" {
		parts = append(parts, msg)
	}
	if hint != "" {
		parts = append(parts, hint)
	}
	return fmt.Errorf("%s", strings.Join(parts, ". "))
}

// serverMessage достаёт текст отказа из двух форм, которые отдаёт api:
// `{"error":{"code","message"}}` у /v1 и `{"detail": …}` у FastAPI-валидации.
func serverMessage(body []byte) string {
	var v struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Detail json.RawMessage `json:"detail"`
	}
	if json.Unmarshal(body, &v) != nil {
		s := strings.TrimSpace(string(body))
		if len(s) > 300 {
			s = s[:300] + "…"
		}
		return s
	}
	if v.Error != nil {
		if v.Error.Code != "" && v.Error.Message != "" {
			return v.Error.Code + ": " + v.Error.Message
		}
		return v.Error.Message + v.Error.Code
	}
	if len(v.Detail) > 0 {
		var s string
		if json.Unmarshal(v.Detail, &s) == nil {
			return s
		}
		var items []struct {
			Loc []any  `json:"loc"`
			Msg string `json:"msg"`
		}
		if json.Unmarshal(v.Detail, &items) == nil {
			var out []string
			for _, it := range items {
				loc := make([]string, 0, len(it.Loc))
				for _, l := range it.Loc {
					if s := fmt.Sprint(l); s != "body" {
						loc = append(loc, s)
					}
				}
				out = append(out, strings.Join(loc, ".")+": "+it.Msg)
			}
			return strings.Join(out, "; ")
		}
		return string(v.Detail)
	}
	return ""
}

func orDefault(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

// check: транспортная ошибка — «api недоступен», не-2xx — apiError.
//
// r — сгенерированный ответ tatnet-go (у всех одинаковые поля Body и
// HTTPResponse, но общего интерфейса для полей нет — отсюда reflect). При
// транспортной ошибке r равен nil и не трогается.
func check(op string, r any, err error) error {
	if err != nil {
		return fmt.Errorf("%s failed: TatNet API is unreachable (%v); retry later", op, err)
	}
	v := reflect.ValueOf(r)
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return fmt.Errorf("%s failed: empty response from TatNet API", op)
		}
		v = v.Elem()
	}
	httpResp, _ := v.FieldByName("HTTPResponse").Interface().(*http.Response)
	body, _ := v.FieldByName("Body").Interface().([]byte)
	if httpResp == nil {
		return fmt.Errorf("%s failed: empty response from TatNet API", op)
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return apiError(op, httpResp, body)
	}
	return nil
}
