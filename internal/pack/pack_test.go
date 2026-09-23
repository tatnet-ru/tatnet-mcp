package pack

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"strings"
	"testing"
)

func untar(t *testing.T, b []byte) map[string]string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	out := map[string]string{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		data, _ := io.ReadAll(tr)
		out[h.Name] = string(data)
	}
}

func TestBuildPacksFilesAndSkipsEnv(t *testing.T) {
	res, err := Build([]File{
		{Path: "index.html", Content: "<h1>hi</h1>"},
		{Path: "./src//app.js", Content: "x"},
		{Path: "img/dot.png", Content: "iVBORw0K", Encoding: "base64"},
		{Path: ".env", Content: "SECRET=1"},
		{Path: "web/.env.local", Content: "SECRET=2"},
		{Path: ".env.example", Content: "SECRET="},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := untar(t, res.Archive)
	if len(got) != 4 || got["index.html"] != "<h1>hi</h1>" || got["src/app.js"] != "x" {
		t.Fatalf("archive content: %v", got)
	}
	if _, ok := got[".env"]; ok {
		t.Fatal(".env must not be packed")
	}
	if strings.Join(res.Skipped, ",") != ".env,web/.env.local" {
		t.Fatalf("skipped: %v", res.Skipped)
	}
}

func TestBuildIsDeterministic(t *testing.T) {
	a, _ := Build([]File{{Path: "b", Content: "2"}, {Path: "a", Content: "1"}})
	b, _ := Build([]File{{Path: "a", Content: "1"}, {Path: "b", Content: "2"}})
	if !bytes.Equal(a.Archive, b.Archive) {
		t.Fatal("same files in different order must give the same archive")
	}
}

func TestBuildRejectsEscapes(t *testing.T) {
	for _, p := range []string{"../x", "a/../../x", "/etc/passwd", `..\x`, "", ".git/config", "."} {
		if _, err := Build([]File{{Path: p, Content: "x"}}); err == nil {
			t.Errorf("path %q must be rejected", p)
		}
	}
}

func TestBuildLimits(t *testing.T) {
	if _, err := Build(nil); err == nil {
		t.Error("empty file list must fail")
	}
	if _, err := Build([]File{{Path: ".env", Content: "x"}}); err == nil {
		t.Error("only env files must fail, not deploy an empty archive")
	}
	if _, err := Build([]File{{Path: "a", Content: "x"}, {Path: "./a", Content: "y"}}); err == nil {
		t.Error("duplicate path must fail")
	}
	big := strings.Repeat("x", MaxTotalBytes+1)
	if _, err := Build([]File{{Path: "a", Content: big}}); err == nil {
		t.Error("oversized content must fail")
	}
	if _, err := Build([]File{{Path: "a", Content: "!!", Encoding: "base64"}}); err == nil {
		t.Error("bad base64 must fail")
	}
}
