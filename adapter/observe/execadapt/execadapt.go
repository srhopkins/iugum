package execadapt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"

	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/plugin"
)

func init() {
	plugin.RegisterObserver("exec", func(cfg map[string]string) (contract.Observer, error) {
		cmd := fields(cfg["command"])
		if len(cmd) == 0 {
			return nil, fmt.Errorf("iugum: exec observe needs exec.observe command")
		}
		return Observer{cmd: cmd}, nil
	})
}

// Observer is an external helper. Protocol: argv verb, JSON stdin, JSON stdout.
// Verbs: ingest-metrics, query-metrics, ingest-logs, search-logs.
type Observer struct{ cmd []string }

func (Observer) Name() string { return "exec" }

func (o Observer) IngestMetrics(ctx context.Context, samples []contract.Sample) error {
	return o.call(ctx, "ingest-metrics", samples, nil)
}

func (o Observer) QueryMetrics(ctx context.Context, q contract.MetricQuery) ([]contract.Series, error) {
	var out []contract.Series
	if err := o.call(ctx, "query-metrics", q, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (o Observer) IngestLogs(ctx context.Context, recs []contract.Log) error {
	return o.call(ctx, "ingest-logs", recs, nil)
}

func (o Observer) SearchLogs(ctx context.Context, q contract.LogQuery) ([]contract.Log, error) {
	var out []contract.Log
	if err := o.call(ctx, "search-logs", q, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (o Observer) call(ctx context.Context, verb string, in any, out any) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	args := append(append([]string{}, o.cmd[1:]...), verb)
	c := exec.CommandContext(ctx, o.cmd[0], args...)
	c.Stdin = bytes.NewReader(raw)
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr
	if err := c.Run(); err != nil {
		if stderr.Len() > 0 {
			return fmt.Errorf("observe exec: %w: %s", err, stderr.String())
		}
		return err
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(stdout.Bytes(), out)
}

func fields(s string) []string {
	var out []string
	cur := ""
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			if cur != "" {
				out = append(out, cur)
				cur = ""
			}
			continue
		}
		cur += string(s[i])
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}
