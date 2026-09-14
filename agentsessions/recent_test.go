package agentsessions

import (
	"context"
	"fmt"
	"testing"
)

func TestLatestScopeRecencyAndPolicy(t *testing.T) {
	i, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()
	for _, v := range []struct{ project, stamp, text string }{
		{"/work/ffai", "2026-09-10T10:00:00Z", "old"},
		{"/work/FutureFit-ai", "2026-09-10T05:00:00-07:00", "latest scoped"},
		{"/work/personal", "2026-09-10T13:00:00Z", "mentions ffai but unrelated"},
		{"/work/secret", "2026-09-10T14:00:00Z", "denied"},
		{"/work/ffai", "", "undated"},
	} {
		_, err = i.db.Exec(`INSERT INTO transcript_fts(platform,account,session,project,path,line,text,timestamp) VALUES('claude','','session',?,'fixture',1,?,?)`, v.project, v.text, v.stamp)
		if err != nil {
			t.Fatal(err)
		}
	}
	allow := func(h Hit) error {
		if h.Project == "/work/secret" {
			return fmt.Errorf("denied")
		}
		return nil
	}
	for _, tc := range []struct{ scope, want string }{{"ffai", "latest scoped"}, {"FutureFit AI", "latest scoped"}, {"", "mentions ffai but unrelated"}, {"missing", ""}} {
		h, err := i.Latest(context.Background(), tc.scope, allow)
		if err != nil {
			t.Fatal(err)
		}
		if tc.want == "" {
			if h != nil {
				t.Fatal(h)
			}
			continue
		}
		if h == nil || h.Text != tc.want {
			t.Fatalf("%s: %+v", tc.scope, h)
		}
	}
	if _, err = i.Latest(context.Background(), "", nil); err != ErrAuthorizationRequired {
		t.Fatal(err)
	}
}

func TestPreviousMessageAcrossToolLines(t *testing.T) {
	i, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()
	for _, v := range []struct {
		session     string
		line        int
		text, stamp string
	}{{"one", 1, "Working on partner validation", "2026-09-10T10:00:00Z"}, {"two", 500, "unrelated", "2026-09-10T11:00:00Z"}, {"one", 900, "Checking now", "2026-09-10T12:00:00Z"}} {
		_, err = i.db.Exec(`INSERT INTO transcript_fts(platform,account,session,project,path,line,text,timestamp) VALUES('claude','',?,'ffai','fixture',?,?,?)`, v.session, v.line, v.text, v.stamp)
		if err != nil {
			t.Fatal(err)
		}
	}
	allow := func(Hit) error { return nil }
	h, err := i.Latest(context.Background(), "ffai", allow)
	if err != nil {
		t.Fatal(err)
	}
	p, err := i.PreviousMessage(context.Background(), *h, allow)
	if err != nil || p == nil || p.Text != "Working on partner validation" {
		t.Fatal(p, err)
	}
}
