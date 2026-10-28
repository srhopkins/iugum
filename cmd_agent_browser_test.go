package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestBrowserGrantsCLIRequiresHostToolsBeforeRuntime(t *testing.T) {
	var out, err bytes.Buffer
	code := runAgentSessions(context.Background(), nil, []string{"serve", "--browser-grants"}, &out, &err)
	if code != 2 || !strings.Contains(err.String(), "requires host-tools") {
		t.Fatal(code, err.String())
	}
}
