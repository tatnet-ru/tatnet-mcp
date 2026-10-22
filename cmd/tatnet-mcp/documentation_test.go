package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Every discoverable tool needs an entry in the user-facing catalog, including
// restricted tools. This catches the drift that left VM/S3 tools undocumented.
func TestToolCatalogIsDocumented(t *testing.T) {
	raw, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	_, srv := setup(t)
	session, err := connect(t, srv.URL, goodKey)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	cursor := ""
	for {
		params := &mcp.ListToolsParams{Cursor: cursor}
		result, err := session.ListTools(context.Background(), params)
		if err != nil {
			t.Fatal(err)
		}
		for _, tool := range result.Tools {
			if !strings.Contains(string(raw), "`"+tool.Name+"`") {
				t.Errorf("tool %s is missing from README", tool.Name)
			}
		}
		if result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
	}
}
