package policy

import (
	"context"
	"fmt"
	"strings"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
	fileadapter "github.com/casbin/casbin/v2/persist/file-adapter"

	"github.com/srhopkins/iugum/contract"
)

// allow-all with deny override. One row p, *, *, *, allow is the default.
// Add p, <sub>, <obj>, <act>, deny to lock one action (e.g. schedule, add).
const defaultModel = `
[request_definition]
r = sub, obj, act

[policy_definition]
p = sub, obj, act, eft

[policy_effect]
e = some(where (p.eft == allow)) && !some(where (p.eft == deny))

[matchers]
m = (p.sub == "*" || p.sub == r.sub) && (p.obj == "*" || p.obj == r.obj || keyMatch(r.obj, p.obj)) && (p.act == "*" || p.act == r.act)
`

const defaultPolicy = `p, *, *, *, allow
`

// Gate is Casbin behind contract.Policy.
type Gate struct {
	e *casbin.Enforcer
}

func New(modelPath, policyPath string) (*Gate, error) {
	var e *casbin.Enforcer
	var err error
	if modelPath != "" && policyPath != "" {
		e, err = casbin.NewEnforcer(modelPath, fileadapter.NewAdapter(policyPath))
	} else if modelPath == "" && policyPath != "" {
		m, merr := model.NewModelFromString(defaultModel)
		if merr != nil {
			return nil, merr
		}
		e, err = casbin.NewEnforcer(m, fileadapter.NewAdapter(policyPath))
	} else if modelPath != "" {
		e, err = casbin.NewEnforcer(modelPath)
	} else {
		m, merr := model.NewModelFromString(defaultModel)
		if merr != nil {
			return nil, merr
		}
		e, err = casbin.NewEnforcer(m)
		if err == nil {
			for _, line := range strings.Split(strings.TrimSpace(defaultPolicy), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				// p, sub, obj, act, eft
				parts := splitCSV(line)
				if len(parts) >= 5 {
					_, _ = e.AddPolicy(parts[1], parts[2], parts[3], parts[4])
				}
			}
		}
	}
	if err != nil {
		return nil, fmt.Errorf("iugum: casbin: %w", err)
	}
	return &Gate{e: e}, nil
}

func (g *Gate) Enforce(_ context.Context, req contract.Request) error {
	ok, err := g.e.Enforce(req.Sub, req.Obj, req.Act)
	if err != nil {
		return fmt.Errorf("iugum: policy: %w", err)
	}
	if !ok {
		return contract.Denied{Req: req}
	}
	return nil
}

func splitCSV(line string) []string {
	raw := strings.Split(line, ",")
	out := make([]string, 0, len(raw))
	for _, p := range raw {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}
