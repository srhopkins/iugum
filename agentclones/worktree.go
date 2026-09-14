// Package agentclones isolates candidate source changes and binds evaluation to exact content.
package agentclones

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Manifest struct {
	Parent    string            `json:"parent"`
	Base      string            `json:"base"`
	Files     map[string]string `json:"files"`
	CreatedAt time.Time         `json:"created_at"`
}
type Evaluation struct {
	Digest  string    `json:"digest"`
	Passed  bool      `json:"passed"`
	Command []string  `json:"command"`
	Output  string    `json:"output"`
	At      time.Time `json:"at"`
}

func git(ctx context.Context, dir string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var out, errout bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errout
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, errout.String())
	}
	return out.Bytes(), nil
}
func sourceFiles(ctx context.Context, dir string) (map[string]string, error) {
	b, e := git(ctx, dir, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if e != nil {
		return nil, e
	}
	result := map[string]string{}
	for _, p := range strings.Split(string(b), "\x00") {
		if p == "" {
			continue
		}
		sum, e := hashSourceFile(dir, p)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return nil, e
		}
		result[p] = sum
	}
	return result, nil
}
func writeJSON(path string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	f, e := os.CreateTemp(filepath.Dir(path), ".candidate-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), path)
}

// SnapshotWorktree includes the parent's tracked modifications and untracked source,
// while preserving the parent's checkout and avoiding a commit.
func SnapshotWorktree(ctx context.Context, parent, dest string) (Manifest, error) {
	var m Manifest
	root, e := git(ctx, parent, "rev-parse", "--show-toplevel")
	if e != nil {
		return m, e
	}
	parent = strings.TrimSpace(string(root))
	absoluteDest, err := filepath.Abs(dest)
	if err != nil {
		return m, err
	}
	rel, err := filepath.Rel(parent, absoluteDest)
	if err != nil {
		return m, err
	}
	if rel == "." || (!strings.HasPrefix(rel, ".."+string(os.PathSeparator)) && rel != "..") {
		return m, errors.New("candidate source must be outside its parent repository")
	}
	dest = absoluteDest
	head, e := git(ctx, parent, "rev-parse", "HEAD")
	if e != nil {
		return m, e
	}
	if _, e = os.Lstat(dest); !os.IsNotExist(e) {
		return m, errors.New("candidate source destination must not exist")
	}
	before, e := sourceFiles(ctx, parent)
	if e != nil {
		return m, e
	}
	if _, e = git(ctx, parent, "worktree", "add", "--detach", dest, strings.TrimSpace(string(head))); e != nil {
		return m, e
	}
	// A failed copy leaves the worktree for inspection; never remove a potentially edited tree.
	current, e := sourceFiles(ctx, dest)
	if e != nil {
		return m, e
	}
	for p := range current {
		if _, ok := before[p]; !ok {
			full, e := safePath(dest, p)
			if e != nil {
				return m, e
			}
			if e = os.Remove(full); e != nil {
				return m, e
			}
		}
	}
	for p := range before {
		if e = copySourceFile(parent, dest, p); e != nil {
			return m, e
		}
	}
	after, e := sourceFiles(ctx, parent)
	if e != nil {
		return m, e
	}
	if !sameFiles(before, after) {
		return m, errors.New("parent changed during snapshot; candidate must be recreated")
	}
	m = Manifest{Parent: parent, Base: strings.TrimSpace(string(head)), Files: before, CreatedAt: time.Now().UTC()}
	return m, writeJSON(filepath.Join(filepath.Dir(dest), "source-manifest.json"), m)
}
func sameFiles(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
func safePath(root, path string) (string, error) {
	if filepath.IsAbs(path) || path == "" {
		return "", errors.New("candidate path must be relative")
	}
	clean := filepath.Clean(path)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", errors.New("candidate path escapes root")
	}
	for _, p := range strings.Split(filepath.ToSlash(clean), "/") {
		if p == ".git" {
			return "", errors.New("git metadata is not editable")
		}
	}
	st, e := os.Lstat(root)
	if e != nil {
		return "", e
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("candidate root cannot be a symlink")
	}
	root, e = filepath.EvalSymlinks(root)
	if e != nil {
		return "", e
	}
	full := filepath.Join(root, clean)
	cur := root
	for _, p := range strings.Split(clean, string(os.PathSeparator)) {
		cur = filepath.Join(cur, p)
		st, e := os.Lstat(cur)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return "", e
		}
		if st.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("candidate symlinks are not followed")
		}
	}
	return full, nil
}
func Read(root, path string) ([]byte, error) {
	p, e := safePath(root, path)
	if e != nil {
		return nil, e
	}
	st, e := os.Stat(p)
	if e != nil {
		return nil, e
	}
	if st.IsDir() || st.Size() > 8<<20 {
		return nil, errors.New("source file exceeds 8 MiB or is a directory")
	}
	return os.ReadFile(p)
}
func Write(root, path string, data []byte) error {
	if len(data) > 8<<20 {
		return errors.New("source write exceeds 8 MiB")
	}
	p, e := safePath(root, path)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(p), 0700); e != nil {
		return e
	}
	mode := os.FileMode(0644)
	if st, e := os.Stat(p); e == nil {
		mode = st.Mode().Perm()
	}
	f, e := os.CreateTemp(filepath.Dir(p), ".source-*")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if e = f.Chmod(mode); e != nil {
		f.Close()
		return e
	}
	if _, e = f.Write(data); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), p)
}
func Digest(ctx context.Context, root string) (string, error) {
	files, e := sourceFiles(ctx, root)
	if e != nil {
		return "", e
	}
	keys := []string{}
	for k := range files {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		fmt.Fprintf(h, "%s\x00%s\x00", k, files[k])
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Evaluate only runs Go checks, not arbitrary shell text. Candidate code is trusted
// only within the policy-authorized development environment, never as production.
func Evaluate(ctx context.Context, root string, args []string) (Evaluation, error) {
	var result Evaluation
	if len(args) == 0 || (args[0] != "test" && args[0] != "vet" && args[0] != "build") {
		return result, errors.New("evaluation supports go test, go vet, or go build")
	}
	for _, a := range args {
		if strings.HasPrefix(a, "-exec") || strings.HasPrefix(a, "-toolexec") {
			return result, errors.New("execution overrides are not supported")
		}
	}
	before, e := Digest(ctx, root)
	if e != nil {
		return result, e
	}
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0")
	b, e := cmd.CombinedOutput()
	if len(b) > 32000 {
		b = b[len(b)-32000:]
	}
	after, de := Digest(ctx, root)
	result = Evaluation{Digest: after, Passed: e == nil && de == nil && before == after, Command: append([]string{"go"}, args...), Output: string(b), At: time.Now().UTC()}
	if de != nil {
		return result, de
	}
	if se := writeJSON(filepath.Join(filepath.Dir(root), "evaluation.json"), result); se != nil {
		return result, se
	}
	return result, nil
}

// PromotionPlan refuses parent drift and stale/failed evaluations. It returns
// paths for human review; applying them requires a separate approval in the host.
func PromotionPlan(ctx context.Context, root string) (Manifest, []string, error) {
	var m Manifest
	var ev Evaluation
	b, e := os.ReadFile(filepath.Join(filepath.Dir(root), "source-manifest.json"))
	if e != nil {
		return m, nil, e
	}
	if e = json.Unmarshal(b, &m); e != nil {
		return m, nil, e
	}
	b, e = os.ReadFile(filepath.Join(filepath.Dir(root), "evaluation.json"))
	if e != nil {
		return m, nil, e
	}
	if e = json.Unmarshal(b, &ev); e != nil {
		return m, nil, e
	}
	digest, e := Digest(ctx, root)
	if e != nil {
		return m, nil, e
	}
	if !ev.Passed || ev.Digest != digest {
		return m, nil, errors.New("candidate has no passing evaluation for its current content")
	}
	parent, e := sourceFiles(ctx, m.Parent)
	if e != nil {
		return m, nil, e
	}
	if !sameFiles(parent, m.Files) {
		return m, nil, errors.New("parent changed since clone creation; rebase the candidate before promotion")
	}
	candidate, e := sourceFiles(ctx, root)
	if e != nil {
		return m, nil, e
	}
	changed := map[string]bool{}
	for k, v := range candidate {
		if parent[k] != v {
			changed[k] = true
		}
	}
	for k := range parent {
		if _, ok := candidate[k]; !ok {
			changed[k] = true
		}
	}
	paths := []string{}
	for p := range changed {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return m, paths, nil
}

func hashSourceFile(root, p string) (string, error) {
	full := filepath.Join(root, p)
	st, e := os.Lstat(full)
	if e != nil {
		return "", e
	}
	if st.Mode()&os.ModeSymlink != 0 {
		target, e := os.Readlink(full)
		if e != nil {
			return "", e
		}
		sum := sha256.Sum256([]byte("symlink:" + target))
		return hex.EncodeToString(sum[:]), nil
	}
	full, e = safePath(root, p)
	if e != nil {
		return "", e
	}
	f, e := os.Open(full)
	if e != nil {
		return "", e
	}
	defer f.Close()
	h := sha256.New()
	fmt.Fprintf(h, "mode:%o\x00", st.Mode().Perm())
	if _, e = io.Copy(h, f); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func copySourceFile(from, to, p string) error {
	src := filepath.Join(from, p)
	st, e := os.Lstat(src)
	if e != nil {
		return e
	}
	if st.Mode()&os.ModeSymlink != 0 {
		target, e := os.Readlink(src)
		if e != nil {
			return e
		}
		dest := filepath.Join(to, p)
		if old, e := os.Readlink(dest); e == nil && old == target {
			return nil
		}
		if _, e = os.Lstat(dest); e == nil {
			return errors.New("changed symlink requires explicit reconciliation")
		}
		if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
			return e
		}
		return os.Symlink(target, dest)
	}
	src, e = safePath(from, p)
	if e != nil {
		return e
	}
	dest, e := safePath(to, p)
	if e != nil {
		return e
	}
	if e = os.MkdirAll(filepath.Dir(dest), 0700); e != nil {
		return e
	}
	in, e := os.Open(src)
	if e != nil {
		return e
	}
	defer in.Close()
	out, e := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, st.Mode().Perm())
	if e != nil {
		return e
	}
	_, e = io.Copy(out, in)
	closeErr := out.Close()
	if e != nil {
		return e
	}
	return closeErr
}

// Promote applies a reviewed candidate only if both its evaluation and parent
// baseline still match. Backups are written before the first parent edit.
func Promote(ctx context.Context, root, expectedParent string) ([]string, error) {
	m, paths, e := PromotionPlan(ctx, root)
	if e != nil {
		return nil, e
	}
	expectedParent, e = filepath.EvalSymlinks(expectedParent)
	if e != nil {
		return nil, e
	}
	if m.Parent != expectedParent {
		return nil, errors.New("promotion parent differs from approved parent")
	}
	backup := filepath.Join(filepath.Dir(root), "promotion-backup-"+time.Now().UTC().Format("20060102T150405.000000000"))
	if e = os.Mkdir(backup, 0700); e != nil {
		return nil, e
	}
	type saved struct {
		Path   string
		Exists bool
		Mode   os.FileMode
		Data   []byte
	}
	originals := []saved{}
	replacements := map[string][]byte{}
	for _, p := range paths {
		if _, e = safePath(m.Parent, p); e != nil {
			return nil, e
		}
		v, re := Read(root, p)
		if re != nil && !os.IsNotExist(re) {
			return nil, re
		}
		if re == nil {
			replacements[p] = v
		}
		old, re := Read(m.Parent, p)
		if re != nil && !os.IsNotExist(re) {
			return nil, re
		}
		s := saved{Path: p, Exists: re == nil, Data: old, Mode: 0644}
		if s.Exists {
			st, _ := os.Stat(filepath.Join(m.Parent, p))
			s.Mode = st.Mode().Perm()
			if e = Write(backup, p, old); e != nil {
				return nil, e
			}
		}
		originals = append(originals, s)
	}
	if e = writeJSON(filepath.Join(backup, "manifest.json"), map[string]any{"parent": m.Parent, "paths": paths, "created_at": time.Now().UTC()}); e != nil {
		return nil, e
	}
	// Recheck immediately before applying; parent drift must never be overwritten.
	if _, _, e = PromotionPlan(ctx, root); e != nil {
		return nil, e
	}
	applied := 0
	rollback := func() {
		for i := applied - 1; i >= 0; i-- {
			s := originals[i]
			if s.Exists {
				_ = Write(m.Parent, s.Path, s.Data)
				_ = os.Chmod(filepath.Join(m.Parent, s.Path), s.Mode)
			} else {
				_ = os.Remove(filepath.Join(m.Parent, s.Path))
			}
		}
	}
	for _, s := range originals {
		if e = ctx.Err(); e != nil {
			rollback()
			return nil, e
		}
		if v, ok := replacements[s.Path]; ok {
			e = Write(m.Parent, s.Path, v)
			if e == nil {
				st, se := os.Stat(filepath.Join(root, s.Path))
				if se != nil {
					e = se
				} else {
					e = os.Chmod(filepath.Join(m.Parent, s.Path), st.Mode().Perm())
				}
			}
		} else {
			e = os.Remove(filepath.Join(m.Parent, s.Path))
		}
		if e != nil {
			applied++
			rollback()
			return nil, fmt.Errorf("promotion failed; attempted rollback, backups at %s: %w", backup, e)
		}
		applied++
	}
	return paths, nil
}
