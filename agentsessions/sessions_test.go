package agentsessions

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
)

func TestSessionsContentIdentityPolicyAndReadOnlyTranscript(t *testing.T) {
	i, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()
	ctx := context.Background()
	for _, row := range []struct {
		platform, path, text string
		line                 int
	}{{"codex", "/one", "Same title", 1}, {"codex", "/one", "body-only needle", 2}, {"claude", "/two", "Same title", 1}, {"claude", "/two", "body-only needle", 2}, {"cursor", "/denied", "body-only needle", 1}, {"opencode", "/three", "body-only needle", 1}} {
		_, err = i.db.Exec(`INSERT INTO transcript_fts(platform,account,session,project,path,line,text,timestamp) VALUES(?,?,?,?,?,?,?,?)`, row.platform, "", "same-id", "project", row.path, row.line, row.text, "")
		if err != nil {
			t.Fatal(err)
		}
	}
	auth := func(h Hit) error {
		if h.Path == "/denied" {
			return errors.New("denied")
		}
		return nil
	}
	hits, err := i.Sessions(ctx, "needle", auth)
	if err != nil || len(hits) != 3 {
		t.Fatalf("%+v %v", hits, err)
	}
	if hits[0].ID == hits[1].ID {
		t.Fatal("provider identities merged")
	}
	for _, h := range hits {
		rows, truncated, err := i.Transcript(ctx, h.ID, auth)
		if err != nil || truncated || len(rows) == 0 {
			t.Fatalf("%+v %v", rows, err)
		}
		for _, r := range rows {
			if r.Platform != h.Provider || r.Session != h.SessionID {
				t.Fatal("cross-session read")
			}
		}
	}
	hits, err = i.Sessions(ctx, `" OR "`, auth)
	if err != nil || len(hits) != 0 {
		t.Fatal(hits, err)
	}
	if _, err = i.Sessions(ctx, "", nil); err == nil {
		t.Fatal("missing authorization accepted")
	}
}

func TestSessionMetadataTitlesAndDatabaseMessageGrouping(t *testing.T) {
	for _, platform := range []string{"cursor", "opencode"} {
		t.Run(platform, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "source.db")
			if platform == "cursor" {
				path += ".vscdb"
			}
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if platform == "cursor" {
				_, err = db.Exec(`CREATE TABLE cursorDiskKV(key TEXT,value BLOB);INSERT INTO cursorDiskKV VALUES('composerData:same-id','{"name":"renamed unicorn"}')`)
			} else {
				_, err = db.Exec(`CREATE TABLE session(id TEXT,title TEXT);INSERT INTO session VALUES('same-id','renamed unicorn')`)
			}
			if err != nil {
				t.Fatal(err)
			}
			db.Close()
			i, err := Open(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer i.Close()
			for line, text := range []string{"ordinary first message", "later needle"} {
				source := path + "#bubble=" + fmt.Sprint(line)
				if platform == "opencode" {
					source = path + "#part=" + fmt.Sprint(line)
				}
				_, err = i.db.Exec(`INSERT INTO transcript_fts(platform,account,session,project,path,line,text,timestamp) VALUES(?,?,?,?,?,?,?,?)`, platform, "", "same-id", "project", source, line, text, "")
				if err != nil {
					t.Fatal(err)
				}
			}
			auth := func(Hit) error { return nil }
			hits, err := i.Sessions(context.Background(), "unicorn", auth)
			if err != nil || len(hits) != 1 || hits[0].Title != "renamed unicorn" {
				t.Fatal(hits, err)
			}
			transcript, _, err := i.Transcript(context.Background(), hits[0].ID, auth)
			if err != nil || len(transcript) != 2 {
				t.Fatal(transcript, err)
			}
		})
	}
}
