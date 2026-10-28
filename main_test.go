package main

import (
	"reflect"
	"testing"
)

func TestExtractViewerConfigFlag(t *testing.T) {
	rest, file, err := extractViewerConfigFlag([]string{"--projects", "projects.yaml", "--viewer-config=lanes.yaml", "--read-only"})
	if err != nil || file != "lanes.yaml" || !reflect.DeepEqual(rest, []string{"--projects", "projects.yaml", "--read-only"}) {
		t.Fatalf("%v %q %v", rest, file, err)
	}
	if _, _, err := extractViewerConfigFlag([]string{"--viewer-config", "--read-only"}); err == nil {
		t.Fatal("expected missing file error")
	}
}
