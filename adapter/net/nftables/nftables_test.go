package nftables

import (
	"context"
	"testing"

	"github.com/srhopkins/iugum/contract"
)

func fixture() contract.NetRules {
	return contract.NetRules{
		Default: contract.NetDefault{In: "deny", Out: "allow"},
		Rules: []contract.NetRule{
			{Name: "wiki-in", Dir: "in", Proto: "tcp", Port: 3000, Src: "203.0.113.0/24", Action: "allow"},
			{Name: "observe-in", Dir: "in", Proto: "tcp", Port: 3848, Action: "allow"},
			{Name: "no-smtp-out", Dir: "out", Proto: "tcp", Port: 25, Action: "deny"},
		},
	}
}

const golden = `table inet iugum {}
flush table inet iugum
table inet iugum {
	chain input {
		type filter hook input priority 0; policy drop;
		iif lo accept
		ip saddr 203.0.113.0/24 tcp dport 3000 accept comment "wiki-in"
		tcp dport 3848 accept comment "observe-in"
	}
	chain output {
		type filter hook output priority 0; policy accept;
		tcp dport 25 drop comment "no-smtp-out"
	}
}
`

func TestPlanGolden(t *testing.T) {
	lines, err := (&Net{}).Plan(context.Background(), fixture())
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 1 {
		t.Fatalf("want one ruleset, got %d", len(lines))
	}
	if lines[0] != golden {
		t.Fatalf("got:\n%s\nwant:\n%s", lines[0], golden)
	}
}
