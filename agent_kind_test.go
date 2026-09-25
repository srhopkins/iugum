package main

import (
	"strings"
	"testing"
)

func TestKindPresetSelkiesHomelab(t *testing.T) {
	af, err := kindPreset("selkies", "homelab", "chrome1")
	if err != nil {
		t.Fatalf("kindPreset: %v", err)
	}
	if af.Network.External != "proxy" {
		t.Errorf("Network.External = %q, want %q", af.Network.External, "proxy")
	}
	if af.User != "" {
		t.Errorf("User = %q, want empty", af.User)
	}
	if !containsStr(af.Volumes, "chrome1-config:/config") {
		t.Errorf("Volumes = %v, want to contain %q", af.Volumes, "chrome1-config:/config")
	}
	if af.ShmSize != "1g" {
		t.Errorf("ShmSize = %q, want %q", af.ShmSize, "1g")
	}
	if !containsSubstr(af.Labels, "tls.passthrough=true") {
		t.Errorf("Labels = %v, want a tls.passthrough=true label", af.Labels)
	}
	if !containsSubstr(af.Labels, "loadbalancer.server.port=3001") {
		t.Errorf("Labels = %v, want the Selkies port 3001 label", af.Labels)
	}
}

func TestKindPresetSelkiesDockerMac(t *testing.T) {
	af, err := kindPreset("selkies", "docker-mac", "chrome1")
	if err != nil {
		t.Fatalf("kindPreset: %v", err)
	}
	if af.Network.External != "agentbox" {
		t.Errorf("Network.External = %q, want %q", af.Network.External, "agentbox")
	}
	if !containsStr(af.Labels, "caddy_0=http://chrome1.localhost") {
		t.Errorf("Labels = %v, want caddy_0=http://chrome1.localhost", af.Labels)
	}
	if af.User != "" {
		t.Errorf("User = %q, want empty", af.User)
	}
	for _, l := range af.Labels {
		if strings.HasPrefix(l, "traefik.") {
			t.Errorf("Labels = %v, want no traefik labels on docker-mac", af.Labels)
		}
	}
}

func TestKindPresetUnknownDefaultsToHomelab(t *testing.T) {
	for _, ctx := range []string{"", "bogus"} {
		af, err := kindPreset("selkies", ctx, "chrome1")
		if err != nil {
			t.Fatalf("kindPreset(%q): %v", ctx, err)
		}
		if af.Network.External != "proxy" {
			t.Errorf("dockerContext %q: Network.External = %q, want %q", ctx, af.Network.External, "proxy")
		}
	}
}

func TestKindPresetUnknownKind(t *testing.T) {
	_, err := kindPreset("nonesuch", "homelab", "chrome1")
	if err == nil {
		t.Fatal("expected error for unknown kind")
	}
	if !strings.Contains(err.Error(), "selkies") {
		t.Errorf("error %q should mention %q", err.Error(), "selkies")
	}
}

func containsStr(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func containsSubstr(list []string, want string) bool {
	for _, s := range list {
		if strings.Contains(s, want) {
			return true
		}
	}
	return false
}
