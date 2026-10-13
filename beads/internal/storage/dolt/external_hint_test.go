package dolt

import (
	"strings"
	"testing"
)

func TestExternalServerHintNeverSuggestsStart(t *testing.T) {
	got := ExternalServerHint("db.example.com", 3307)
	want := "The shared Dolt server at db.example.com:3307 is unreachable. Check the server. Do not run bd dolt start here."
	if got != want {
		t.Fatalf("hint = %q, want %q", got, want)
	}
	if strings.Contains(got, "Try:") {
		t.Fatalf("hint must not tell the user to start a server: %q", got)
	}
}

func TestIsExternalServerRemoteHost(t *testing.T) {
	if !isExternalServer(&Config{ServerHost: "db.example.com"}, t.TempDir()) {
		t.Fatal("remote host must count as external")
	}
}
