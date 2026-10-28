package agentcore

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSubscriptionOwnedImageAttachments(t *testing.T) {
	for _, failure := range []bool{false, true} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			dir := t.TempDir()
			record := filepath.Join(dir, "attachment")
			copied := filepath.Join(dir, "copy.png")
			prompt := filepath.Join(dir, "prompt")
			body := fmt.Sprintf(`while [ "$#" -gt 0 ]; do
 case "$1" in --image) shift; image="$1";; -o) shift; output="$1";; esac
 shift
done
[ -n "$image" ] || exit 31
[ "$(stat -f %%Lp "$image" 2>/dev/null || stat -c %%a "$image")" = 600 ] || exit 32
[ "$(stat -f %%Lp "$(dirname "$image")" 2>/dev/null || stat -c %%a "$(dirname "$image")")" = 700 ] || exit 33
printf '%%s' "$image" > '%s'
cp "$image" '%s'
cat > '%s'
`, record, copied, prompt)
			if failure {
				body += "exit 17\n"
			} else {
				body += "printf 'answer' > \"$output\"\nprintf '%s\\n' '{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}'\n"
			}
			// Exercise the actual process adapter with a local fake executable only.
			binary := fakeCLI(t, body)
			cfg := config("http://localhost:1")
			cfg.SubscriptionVisionModels = []string{"vision-model"}
			r, _ := New(cfg, "", gate{})
			png := imagePNG(t)
			_, err := r.RunSubscription(context.Background(), RunRequest{Actor: "chief", Messages: []Message{{Role: "user", Content: "original human anchor"}, {Role: "user", Content: "UNTRUSTED observation metadata", Images: []Image{{MIMEType: "image/png", Data: png}}}}}, binary, "vision-model")
			if (err != nil) != failure {
				t.Fatalf("failure=%v error=%v", failure, err)
			}
			path, e := os.ReadFile(record)
			if e != nil {
				t.Fatal(e)
			}
			if _, e = os.Stat(string(path)); !os.IsNotExist(e) {
				t.Fatal("attachment survived invocation", e)
			}
			got, _ := os.ReadFile(copied)
			if !bytes.Equal(got, png) {
				t.Fatal("PNG bytes changed")
			}
			p, _ := os.ReadFile(prompt)
			if !strings.Contains(string(p), "original human anchor") || strings.Contains(string(p), string(path)) {
				t.Fatal("anchor lost or path leaked into prompt")
			}
		})
	}
}
func TestSubscriptionImageOptInBeforeProcess(t *testing.T) {
	r, _ := New(config("http://localhost:1"), "", gate{})
	_, err := r.RunSubscription(context.Background(), RunRequest{Actor: "chief", Messages: []Message{{Role: "user", Images: []Image{{MIMEType: "image/png", Data: imagePNG(t)}}}}}, "/must/not/start", "unknown")
	if err == nil || !strings.Contains(err.Error(), "opt-in") {
		t.Fatal(err)
	}
}

func TestSubscriptionImageCancellationCleansFiles(t *testing.T) {
	record := filepath.Join(t.TempDir(), "attachment")
	binary := fakeCLI(t, fmt.Sprintf(`while [ "$#" -gt 0 ]; do
 if [ "$1" = --image ]; then shift; printf '%%s' "$1" > '%s'; fi
 shift
done
exec sleep 30
`, record))
	cfg := config("http://localhost:1")
	cfg.SubscriptionVisionModels = []string{"vision"}
	r, _ := New(cfg, "", gate{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := r.RunSubscription(ctx, RunRequest{Actor: "chief", Messages: []Message{{Role: "user", Content: "anchor", Images: []Image{{MIMEType: "image/png", Data: imagePNG(t)}}}}}, binary, "vision")
		done <- err
	}()
	var path []byte
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		path, _ = os.ReadFile(record)
		if len(path) > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if len(path) == 0 {
		t.Fatal("fake CLI never received attachment")
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancellation succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not stop process")
	}
	if _, err := os.Stat(string(path)); !os.IsNotExist(err) {
		t.Fatal("cancelled attachment survived", err)
	}
}
