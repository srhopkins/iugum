package agentexec

import (
	"context"
	"errors"
	"testing"
)

func TestUnsupportedFailsClosed(t *testing.T) {
	_, err := (Unsupported{}).Execute(context.Background(), Request{Command: []string{"true"}})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatal(err)
	}
}
