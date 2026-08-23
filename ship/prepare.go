// Package ship writes reviewable git publish files. It does not push.
package ship

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Opts is how an agent or person asks for a prepare step.
type Opts struct {
	Repo     string
	Base     string
	Head     string
	Title    string
	Body     string
	BodyFile string
	Stdin    io.Reader
}

// Result is the two files to review, then run.
type Result struct {
	Kind string // push or pr
	Dir  string
	MD   string
	SH   string
}

// Prepare writes push.md+push.sh when origin has no branch.
// It writes pr.md+create.sh when a base branch exists.
func Prepare(opts Opts) (Result, error) {
	repo, err := absRepo(opts.Repo)
	if err != nil {
		return Result{}, err
	}
	base := opts.Base
	if base == "" {
		base = "main"
	}
	head := opts.Head
	if head == "" {
		out, err := git(repo, "rev-parse", "--abbrev-ref", "HEAD")
		if err != nil {
			return Result{}, err
		}
		head = strings.TrimSpace(out)
	}
	body, err := readBody(opts)
	if err != nil {
		return Result{}, err
	}
	title := strings.TrimSpace(opts.Title)
	if title == "" {
		title = firstLine(body)
	}
	if title == "" {
		title = "WIP"
	}

	stamp := time.Now().UTC().Format("20060102T150405Z")
	dir := filepath.Join(repo, ".iugum", "prepare", stamp)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Result{}, err
	}

	empty, err := originEmpty(repo)
	if err != nil {
		return Result{}, err
	}
	if empty {
		return writePush(dir, repo, base, head, title, body)
	}
	return writePR(dir, repo, base, head, title, body)
}

func absRepo(repo string) (string, error) {
	if repo == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		repo = wd
	}
	return filepath.Abs(repo)
}

func readBody(opts Opts) (string, error) {
	if opts.Body != "" {
		return opts.Body, nil
	}
	if opts.BodyFile != "" {
		b, err := os.ReadFile(opts.BodyFile)
		return string(b), err
	}
	if opts.Stdin != nil {
		st, _ := opts.Stdin.(*os.File)
		if st != nil {
			info, err := st.Stat()
			if err == nil && (info.Mode()&os.ModeCharDevice) == 0 {
				b, err := io.ReadAll(opts.Stdin)
				return string(b), err
			}
		} else {
			b, err := io.ReadAll(opts.Stdin)
			return string(b), err
		}
	}
	return "", nil
}

func originEmpty(repo string) (bool, error) {
	if err := gitOK(repo, "remote", "get-url", "origin"); err != nil {
		return false, fmt.Errorf("ship: no origin remote")
	}
	out, err := git(repo, "ls-remote", "--heads", "origin")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "", nil
}

func writePush(dir, repo, base, head, title, body string) (Result, error) {
	md := filepath.Join(dir, "push.md")
	sh := filepath.Join(dir, "push.sh")
	text := fmt.Sprintf("# First push: %s\n\n"+
		"The remote has no branch yet. A pull request needs a base branch.\n"+
		"This script pushes `%s` to `origin` as `%s`.\n"+
		"There is no pull request to open.\n\n"+
		"%s\n", title, head, base, strings.TrimSpace(body))
	if err := os.WriteFile(md, []byte(text), 0o644); err != nil {
		return Result{}, err
	}
	script := fmt.Sprintf("#!/bin/sh\nset -eu\ncd %q\ngit push -u origin HEAD:%s\n", repo, shellArg(base))
	if err := os.WriteFile(sh, []byte(script), 0o755); err != nil {
		return Result{}, err
	}
	return Result{Kind: "push", Dir: dir, MD: md, SH: sh}, nil
}

func writePR(dir, repo, base, head, title, body string) (Result, error) {
	md := filepath.Join(dir, "pr.md")
	sh := filepath.Join(dir, "create.sh")
	if strings.TrimSpace(body) == "" {
		body = title
	}
	if err := os.WriteFile(md, []byte(body+"\n"), 0o644); err != nil {
		return Result{}, err
	}
	script := fmt.Sprintf("#!/bin/sh\nset -eu\ncd %q\n"+
		"git push -u origin HEAD\n"+
		"gh pr create --base %s --head %s --title %s --body-file %s\n",
		repo, shellArg(base), shellArg(head), shellArg(title), shellArg(md))
	if err := os.WriteFile(sh, []byte(script), 0o755); err != nil {
		return Result{}, err
	}
	return Result{Kind: "pr", Dir: dir, MD: md, SH: sh}, nil
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimPrefix(s, "# ")
}

func git(repo string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(buf.String()))
	}
	return buf.String(), nil
}

func gitOK(repo string, args ...string) error {
	_, err := git(repo, args...)
	return err
}

func shellArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
