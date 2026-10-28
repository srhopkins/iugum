package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/srhopkins/iugum/agentcore"
)

// A single private atomic snapshot is diagnostic evidence only. Nothing from it
// enters model context, conversation text, tool output, or execution decisions.
func persistNativeEditorDiagnostic(dataDir string, diagnostic agentcore.EditorTurnDiagnostic) error {
	data, e := json.Marshal(diagnostic)
	if e != nil || len(data) > agentcore.MaxEditorDiagnosticBytes {
		return errors.New("editor diagnostic size limit")
	}
	dir := filepath.Join(dataDir, "editor-diagnostics")
	if e = os.MkdirAll(dir, 0700); e != nil {
		return errors.New("editor diagnostic directory unavailable")
	}
	info, e := os.Lstat(dir)
	if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return errors.New("editor diagnostic directory is not private")
	}
	file, e := os.CreateTemp(dir, ".latest-*")
	if e != nil {
		return errors.New("editor diagnostic temporary file unavailable")
	}
	defer os.Remove(file.Name())
	if e = file.Chmod(0600); e != nil {
		file.Close()
		return errors.New("editor diagnostic permissions unavailable")
	}
	if _, e = file.Write(data); e != nil {
		file.Close()
		return errors.New("editor diagnostic write failed")
	}
	if e = file.Close(); e != nil {
		return errors.New("editor diagnostic close failed")
	}
	if e = os.Rename(file.Name(), filepath.Join(dir, "latest.json")); e != nil {
		return errors.New("editor diagnostic replace failed")
	}
	return nil
}

func recordNativeEditorTurn(runtime *agentcore.Runtime, request agentcore.RunRequest, result agentcore.RunResult, tools []agentcore.Tool, dataDir string, cause error, errout io.Writer) {
	if e := persistNativeEditorDiagnostic(dataDir, runtime.EditorTurnDiagnostics(request, result, tools, cause != nil, cause)); e != nil && errout != nil {
		fmt.Fprintln(errout, "Native editor diagnostic snapshot unavailable.")
	}
}
