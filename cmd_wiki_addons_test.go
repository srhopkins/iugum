package main

import (
	"bytes"
	"context"
	"github.com/srhopkins/iugum/app"
	"github.com/srhopkins/iugum/contract"
	"github.com/srhopkins/iugum/policy"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestWikiAddonsArguments(t *testing.T) {
	for _, args := range [][]string{{"--addons", "config.yaml", "-p", "3880", "notes"}, {"-p", "3880", "--addons=config.yaml", "notes"}} {
		rest, path, err := extractWikiAddons(args)
		if err != nil || path != "config.yaml" || !reflect.DeepEqual(rest, []string{"-p", "3880", "notes"}) {
			t.Fatalf("%v %s %v", rest, path, err)
		}
	}
	for _, args := range [][]string{{"--addons"}, {"--addons="}} {
		if _, _, err := extractWikiAddons(args); err == nil {
			t.Fatal("accepted missing config")
		}
	}
}

func TestWikiRejectsServedPrivateConfiguration(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "addons.yaml")
	os.WriteFile(path, []byte("chat: false\nsearch: false\n"), 0600)
	gate, e := policy.New("", "")
	if e != nil {
		t.Fatal(e)
	}
	a := &app.App{Gate: gate, Actor: "test"}
	var output bytes.Buffer
	if code := runWikiAddons(context.Background(), a, contract.WikiOpts{Space: root, Host: "127.0.0.1", Port: 3999}, path, &output); code != 1 {
		t.Fatal(code)
	}
	if !bytes.Contains(output.Bytes(), []byte("outside the served wiki")) {
		t.Fatal(output.String())
	}
}
