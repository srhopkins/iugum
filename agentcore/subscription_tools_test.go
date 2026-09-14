package agentcore

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func decisionCLI(t *testing.T, decision string) string {
	return fakeCLI(t, `
while [ "$#" -gt 0 ]; do
 if [ "$1" = "-o" ]; then shift; output="$1"; fi
 shift
done
cat >/dev/null
cat > "$output" <<'ANSWER'
`+decision+`
ANSWER
printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":5}}'
`)
}
func TestSubscriptionToolsTwoSteps(t *testing.T) {
	binary := fakeCLI(t, `
while [ "$#" -gt 0 ]; do
 if [ "$1" = "-o" ]; then shift; output="$1"; fi
 shift
done
prompt=$(cat)
case "$prompt" in
 *"Untrusted tool result"*) printf '%s' '{"reply":"Found the evidence.","tool_calls":[]}' > "$output";;
 *) printf '%s' '{"reply":"","tool_calls":[{"name":"search","arguments":{"query":"work"}}]}' > "$output";;
esac
printf '%s\n' '{"type":"turn.completed","usage":{"input_tokens":10,"output_tokens":5}}'
`)
	r, _ := New(config("http://localhost:1"), "", gate{})
	authorized, ran := 0, 0
	out, err := r.RunSubscriptionTools(context.Background(), RunRequest{Actor: "chief", Messages: []Message{{Role: "user", Content: "find work"}}}, binary, "test", []Tool{{Name: "search", Authorize: func(context.Context, json.RawMessage) error { authorized++; return nil }, Execute: func(context.Context, json.RawMessage) (string, error) { ran++; return "work evidence", nil }}})
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "Found the evidence." || authorized != 1 || ran != 1 || len(r.Usage()) != 2 {
		t.Fatal(out, authorized, ran, r.Usage())
	}
}
func TestSubscriptionToolsPolicyDenial(t *testing.T) {
	binary := decisionCLI(t, `{"reply":"","tool_calls":[{"name":"write","arguments":{}}]}`)
	r, _ := New(config("http://localhost:1"), "", gate{deny: "agent/tool"})
	ran := false
	_, err := r.RunSubscriptionTools(context.Background(), RunRequest{Actor: "chief"}, binary, "test", []Tool{{Name: "write", Execute: func(context.Context, json.RawMessage) (string, error) { ran = true; return "", nil }}})
	if err == nil || !strings.Contains(err.Error(), "policy denied") || ran {
		t.Fatal(err, ran)
	}
}
func TestSubscriptionToolsMalformedBatch(t *testing.T) {
	for _, decision := range []string{`not json`, `{"reply":"","tool_calls":[{"name":"write","arguments":{}},{"name":"write","arguments":null}]}`, `{"reply":"","tool_calls":[{"name":"write","arguments":{}}]} trailing`} {
		t.Run(decision, func(t *testing.T) {
			r, _ := New(config("http://localhost:1"), "", gate{})
			ran := false
			_, err := r.RunSubscriptionTools(context.Background(), RunRequest{Actor: "chief"}, decisionCLI(t, decision), "test", []Tool{{Name: "write", Execute: func(context.Context, json.RawMessage) (string, error) { ran = true; return "", nil }}})
			if err == nil || ran {
				t.Fatal(err, ran)
			}
		})
	}
}
