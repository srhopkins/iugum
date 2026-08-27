package iptables

import "github.com/srhopkins/iugum/contract"

// Fixture is the sample rule set from scripts/dod/iugum-qps.sh.
// Tests in other packages reuse it.
func Fixture() contract.NetRules {
	return contract.NetRules{
		Default: contract.NetDefault{In: "deny", Out: "allow"},
		Rules: []contract.NetRule{
			{Name: "wiki-in", Dir: "in", Proto: "tcp", Port: 3000, Src: "203.0.113.0/24", Action: "allow"},
			{Name: "observe-in", Dir: "in", Proto: "tcp", Port: 3848, Action: "allow"},
			{Name: "no-smtp-out", Dir: "out", Proto: "tcp", Port: 25, Action: "deny"},
		},
	}
}
