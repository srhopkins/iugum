package beadactivity

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Runner executes Beads argv without a shell. It must preserve the caller's
// configured canonical tracker and must not start a local database.
type Runner func(args ...string) ([]byte, error)

func Report(run Runner, actor, id, state, reason string, now time.Time, ttl time.Duration) error {
	return report(run, actor, id, state, reason, "", now, ttl)
}

func report(run Runner, actor, id, state, reason, runID string, now time.Time, ttl time.Duration) error {
	if id == "" {
		return errors.New("an issue id is required")
	}
	raw, err := run("show", id, "--json")
	if err != nil {
		return err
	}
	var issues []struct{ ID, Assignee, Status string }
	if err = json.Unmarshal(raw, &issues); err != nil {
		return fmt.Errorf("read owner: %w", err)
	}
	if len(issues) != 1 || issues[0].ID != id {
		return errors.New("expected exactly the requested issue")
	}
	if issues[0].Status == "closed" && state != "completed" {
		return errors.New("closed work cannot report working or paused")
	}
	signal, err := NewSignal(actor, issues[0].Assignee, state, reason, now, ttl)
	if err != nil {
		return err
	}
	signal.RunID = runID
	b, err := json.Marshal(signal)
	if err != nil {
		return err
	}
	// Only this metadata key changes. Reassignment racing this write cannot
	// make a new owner spin: Project rejects signals from the former assignee.
	_, err = run("--actor", actor, "update", id, "--set-metadata", MetadataKey+"="+string(b))
	return err
}
