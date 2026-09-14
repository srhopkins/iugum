package main

import (
	"github.com/srhopkins/iugum/agentsessions"
	"strings"
	"testing"
)

func TestLatestSessionStatusEvidence(t *testing.T) {
	s := formatLatestSession(agentsessions.Hit{Platform: "claude", Project: "ffai", Session: "session-one", Path: "/transcripts/one.jsonl", Timestamp: "2026-09-10T20:52:38Z", Text: strings.Repeat("é", 3500)})
	for _, want := range []string{"claude", "ffai", "Last recorded message:", "session-one", "/transcripts/one.jsonl", "…"} {
		if !strings.Contains(s, want) {
			t.Fatal(want, s)
		}
	}
	if got := sessionExcerpt(" a\n b  c ", 3); got != "a\n …" {
		t.Fatal(got)
	}
}
