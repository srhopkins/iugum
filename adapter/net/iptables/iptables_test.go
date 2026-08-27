package iptables

import (
	"context"
	"strings"
	"testing"

	"github.com/srhopkins/iugum/contract"
)

const golden = `iptables -F INPUT
iptables -F OUTPUT
iptables -P INPUT DROP
iptables -P OUTPUT ACCEPT
iptables -A INPUT -i lo -j ACCEPT
iptables -A INPUT -p tcp --dport 3000 -s 203.0.113.0/24 -j ACCEPT
iptables -A INPUT -p tcp --dport 3848 -j ACCEPT
iptables -A OUTPUT -p tcp --dport 25 -j DROP`

func TestPlanGolden(t *testing.T) {
	lines, err := (&Net{}).Plan(context.Background(), Fixture())
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(lines, "\n")
	if got != golden {
		t.Fatalf("got:\n%s\nwant:\n%s", got, golden)
	}
}

func TestValidateRejectsBadDir(t *testing.T) {
	r := contract.NetRules{Rules: []contract.NetRule{{Name: "x", Dir: "sideways"}}}
	if _, err := (&Net{}).Plan(context.Background(), r); err == nil {
		t.Fatal("want error for bad dir")
	}
}

func TestValidateRejectsShellChars(t *testing.T) {
	r := contract.NetRules{Rules: []contract.NetRule{{Name: "x", Dir: "in", Src: "1.2.3.4; rm -rf /"}}}
	if _, err := (&Net{}).Plan(context.Background(), r); err == nil {
		t.Fatal("want error for unsafe value")
	}
}
