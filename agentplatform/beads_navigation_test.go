package agentplatform

import (
	"encoding/json"
	"testing"
)

func TestBeadsNavigationValidation(t *testing.T) {
	for _, raw := range []string{`{}`, `{"view":"board","projects":[],"ticket":"iugum-or7d","ticketProject":"iugum"}`, `{"search":"hello & 雪","statuses":["in_review"],"assignees":["worker + one"],"lanes":true,"sort":"updated"}`} {
		if e := validateBeadsNavigation(json.RawMessage(raw)); e != nil {
			t.Errorf("valid %s: %v", raw, e)
		}
	}
	for _, raw := range []string{`null`, `[]`, `{"url":"https://evil"}`, `{"ticket":"../x"}`, `{"ticket":"x?project=evil"}`, `{"ticketProject":"iugum"}`, `{"projects":"iugum"}`, `{"projects":[null]}`, `{"search":null}`, `{"view":"bad"}`, `{"sort":"bad"}`, `{"lanes":"1"}`, `{"search":"bad\nvalue"}`, `{"projects":[""]}`} {
		if e := validateBeadsNavigation(json.RawMessage(raw)); e == nil {
			t.Errorf("accepted invalid %s", raw)
		}
	}
}

func TestHostCallBeadsNavigation(t *testing.T) {
	for _, raw := range []string{`{}`, `{"ticket":"iugum-or7d","ticketProject":"iugum","statuses":["in_review"]}`} {
		q := HostCall{Instance: "12345678-1234-1234-1234-123456789abc", CommandID: "22345678-1234-1234-1234-123456789abc", Operation: "open_beads", Args: json.RawMessage(raw)}
		if e := validateHostCall(q); e != nil {
			t.Errorf("valid host call: %v", e)
		}
	}
	q := HostCall{Instance: "12345678-1234-1234-1234-123456789abc", CommandID: "22345678-1234-1234-1234-123456789abc", Operation: "open_beads", Args: json.RawMessage(`{"url":"https://evil"}`)}
	if e := validateHostCall(q); e == nil {
		t.Fatal("accepted URL override")
	}
}
