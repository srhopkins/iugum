package main

import (
	"bytes"
	"context"
	"testing"
)

func TestAgentSpeechRelayRequiresExplicitDestinationAndLoopback(t *testing.T) {
	for _, args := range [][]string{{}, {"--listen", "0.0.0.0:8766", "--upstream", "http://127.0.0.1:1"}, {"--listen", "localhost:8766", "--upstream", "http://127.0.0.1:1"}, {"--upstream", "http://127.0.0.1:1", "extra"}} {
		var out, err bytes.Buffer
		if code := runAgentSpeechRelay(context.Background(), nil, args, &out, &err); code == 0 {
			t.Fatal("unsafe or incomplete config accepted")
		}
	}
}
func TestAgentSpeechRelayHelpDoesNotStart(t *testing.T) {
	var out, err bytes.Buffer
	if code := runAgentIO(context.Background(), nil, []string{"speech-relay", "--help"}, &out, &err); code != 0 {
		t.Fatal(code)
	}
}
