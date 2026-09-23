// Package pack собирает архив исходников из файлов, которые прислала модель.
//
// У модели в чате (claude.ai, ChatGPT) нет папки на диске — есть только текст,
// который она написала. Этот пакет превращает его в тот же `.tar.gz`, что
// `tatnet deploy` пакует из папки, и дальше путь общий: /v1 deployments.
package pack

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

// Пределы — на РАСПАКОВАННОЕ содержимое. Это код, написанный в разговоре:
// мегабайты — уже подозрительно, а аргументы вызова инструмента проходят через
// контекст модели целиком. Большие проекты выкладываются из git или CLI.
const (
	MaxFiles      = 2000
	MaxTotalBytes = 20 << 20
)

type File struct {
	Path     string `json:"path" jsonschema:"path relative to the project root, forward slashes, e.g. src/index.ts"`
	Content  string `json:"content" jsonschema:"file content"`
	Encoding string `json:"encoding,omitempty" jsonschema:"utf-8 (default) or base64 for binary files such as images"`
}

type Result struct {
	Archive []byte
	Files   int
	Bytes   int
	// Skipped — файлы окружения, которые в архив не кладутся намеренно:
	// переменные задаются set_env, а не случайно вместе с кодом. Так же
	// поступает `tatnet deploy`.
	Skipped []string
}

// Build пакует файлы. Фиксированные mtime/владелец и сортировка делают архив
// детерминированным: одинаковые файлы — одинаковый sha256 у сборки.
func Build(files []File) (Result, error) {
	var res Result
	if len(files) == 0 {
		return res, fmt.Errorf("no files to deploy")
	}
	if len(files) > MaxFiles {
		return res, fmt.Errorf("too many files: %d (limit %d); deploy large projects from a git repository", len(files), MaxFiles)
	}

	type item struct {
		name string
		data []byte
	}
	seen := map[string]bool{}
	var items []item
	for _, f := range files {
		name, err := clean(f.Path)
		if err != nil {
			return res, err
		}
		if seen[name] {
			return res, fmt.Errorf("duplicate file path %q", name)
		}
		seen[name] = true
		if isEnvFile(name) {
			res.Skipped = append(res.Skipped, name)
			continue
		}
		var data []byte
		switch strings.ToLower(f.Encoding) {
		case "", "utf-8", "utf8", "text":
			data = []byte(f.Content)
		case "base64":
			data, err = base64.StdEncoding.DecodeString(f.Content)
			if err != nil {
				return res, fmt.Errorf("%s: invalid base64: %v", name, err)
			}
		default:
			return res, fmt.Errorf("%s: unknown encoding %q (use utf-8 or base64)", name, f.Encoding)
		}
		res.Bytes += len(data)
		if res.Bytes > MaxTotalBytes {
			return res, fmt.Errorf("files exceed %d MiB in total; deploy large projects from a git repository", MaxTotalBytes>>20)
		}
		items = append(items, item{name, data})
	}
	if len(items) == 0 {
		return res, fmt.Errorf("nothing to deploy: only environment files were given; set variables with set_env instead")
	}
	sort.Slice(items, func(i, j int) bool { return items[i].name < items[j].name })

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	epoch := time.Unix(0, 0)
	for _, it := range items {
		mode := int64(0o644)
		if strings.HasSuffix(it.name, ".sh") {
			mode = 0o755
		}
		hdr := &tar.Header{
			Name: it.name, Mode: mode, Size: int64(len(it.data)),
			ModTime: epoch, Typeflag: tar.TypeReg, Format: tar.FormatPAX,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return res, err
		}
		if _, err := tw.Write(it.data); err != nil {
			return res, err
		}
	}
	if err := tw.Close(); err != nil {
		return res, err
	}
	if err := gz.Close(); err != nil {
		return res, err
	}
	res.Archive = buf.Bytes()
	res.Files = len(items)
	return res, nil
}

// clean нормализует путь и отвергает всё, что выходит за корень проекта.
// Распаковывает архив билдер, но архив, пишущий в `../`, не должен даже
// появиться: это не та ошибка, которую можно доверить следующему звену.
func clean(p string) (string, error) {
	raw := strings.ReplaceAll(strings.TrimSpace(p), `\`, "/")
	if raw == "" {
		return "", fmt.Errorf("empty file path")
	}
	if strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("%q: path must be relative to the project root", p)
	}
	for _, seg := range strings.Split(raw, "/") {
		if seg == ".." {
			return "", fmt.Errorf("%q: path must not contain '..'", p)
		}
	}
	name := path.Clean(raw)
	if name == "." || strings.HasPrefix(name, ".git/") || name == ".git" {
		return "", fmt.Errorf("%q: not a deployable file path", p)
	}
	return name, nil
}

// isEnvFile — то же правило, что у CLI: `.env` и `.env.*`, кроме примера.
func isEnvFile(name string) bool {
	base := path.Base(name)
	if base == ".env.example" || base == ".env.sample" {
		return false
	}
	return base == ".env" || strings.HasPrefix(base, ".env.")
}
