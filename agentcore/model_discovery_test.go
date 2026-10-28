package agentcore

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoveryUsesBackendCredentialAndPreservesExplicitProfiles(t *testing.T) {
	t.Setenv("DISCOVERY_TEST_TOKEN", "private-test-value")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer private-test-value" {
			t.Errorf("wrong path or missing credential")
		}
		fmt.Fprint(w, `{"data":[{"id":"existing"},{"id":"vendor/model"},{"id":"vendor/model"}]}`)
	}))
	defer server.Close()
	c := Config{Pools: map[string]Pool{"proxy": {BaseURL: server.URL + "/v1", APIKeyEnv: "DISCOVERY_TEST_TOKEN", DiscoverModels: true}}, Profiles: map[string]Profile{"custom": {Pool: "proxy", Model: "existing", MaxCompletionTokens: 123, InputUSDPerMillion: 2}}}
	if err := DiscoverProfiles(context.Background(), &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Profiles) != 2 || c.Profiles["custom"].MaxCompletionTokens != 123 {
		t.Fatalf("configured profile changed or duplicate imported: %+v", c.Profiles)
	}
	for key, profile := range c.Profiles {
		if key != "custom" && (profile.Model != "vendor/model" || profile.ZeroCost || profile.Vision) {
			t.Fatalf("incorrect discovered profile: %+v", profile)
		}
	}
	if err := DiscoverProfiles(context.Background(), &c); err != nil || len(c.Profiles) != 2 {
		t.Fatalf("not idempotent: %v", err)
	}
}

func TestDiscoveryRejectsRedirectsMalformedAndSecretErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		code       int
	}{
		{"redirect", "", 302}, {"unauthorized", "secret-token", 401},
		{"missing-data", `{}`, 200}, {"malformed", `{"data":[{"id":"bad\nmodel"}]}`, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", "http://example.invalid/steal")
				w.WriteHeader(tc.code)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			if _, err := DiscoverModels(context.Background(), Pool{BaseURL: server.URL}); err == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
	c := Config{Pools: map[string]Pool{"disabled": {BaseURL: "http://unreachable.invalid"}}}
	if err := DiscoverProfiles(context.Background(), &c); err != nil {
		t.Fatal("discovery should be opt in", err)
	}
}
