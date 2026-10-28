package browsergrant

import (
	"strings"
	"testing"
)

func TestProtocolDecodeRejectsAmbiguousJSON(t *testing.T) {
	for _, s := range []string{`{"x":1,"x":2}`, `{"x":1,"\u0078":2}`, `{"x":-0}`, `{"x":9007199254740992}`, `{"version":1.5}`, `{"x":1} {}`, `{"x":"\ud800"}`} {
		if _, e := Decode(strings.NewReader(s)); e == nil {
			t.Fatalf("accepted %s", s)
		}
	}
	m, e := Decode(strings.NewReader(`{"version":2,"tabId":17,"args":{"x":0.5,"y":0}}`))
	if e != nil || m["version"] != 2 || m["tabId"] != int64(17) {
		t.Fatal(m, e)
	}
}
