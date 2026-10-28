package agentcore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSubscriptionOwnsInitialImageAcrossToolStep(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "seen")
	binary := fakeCLI(t, fmt.Sprintf(`while [ "$#" -gt 0 ]; do
 if [ "$1" = -o ]; then shift; output="$1"; fi
 shift
done
cat >/dev/null
if [ -f '%s' ]; then
 printf '%%s' '{"reply":"done","tool_calls":[]}' > "$output"
else
 touch '%s'
 printf '%%s' '{"reply":"","tool_calls":[{"name":"text","arguments":{}}]}' > "$output"
fi
printf '%%s\n' '{"type":"turn.completed","usage":{"input_tokens":1,"output_tokens":1}}'
`, marker, marker))
	cfg := config("http://localhost:1")
	cfg.SubscriptionVisionModels = []string{"vision"}
	r, _ := New(cfg, "", gate{})
	png := imagePNG(t)
	answer, err := r.RunSubscriptionTools(context.Background(), RunRequest{Actor: "test", Messages: []Message{{Role: "user", Content: "original", Images: []Image{{MIMEType: "image/png", Data: png}}}}}, binary, "vision", []Tool{{Name: "text", Execute: func(context.Context, json.RawMessage) (string, error) { png[0] = 0; return "text result", nil }}})
	if err != nil || answer.Text != "done" {
		t.Fatalf("original caller slice was retained across tool step: answer=%s err=%v", answer.Text, err)
	}
	_ = os.ErrNotExist
}
