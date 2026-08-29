package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/srhopkins/iugum/app"
	"github.com/srhopkins/iugum/config"
)

const jobUsage = `Usage: iugum job <ls|add|rm|run>

  ls                         list jobs from jobs.yaml
  add <name> [flags]         add a job and persist it
  rm <name>                  remove a job
  run <name>                 run a job now (does not wait for cron)

  add flags:
    --every 1h               spec @every 1h
    --spec SPEC              cron spec, @every, or @triggered
    --kind exec|http|session (default exec when a command is given, else session)
    --prompt TEXT            kind session: text injected into the standing session
    --url URL                kind http
    -- <command...>          kind exec

  Cron access is allowed by default. Deny it in policy.csv:
    p, *, schedule, add, deny
`

func runJob(ctx context.Context, a *app.App, args []string) int {
	return runJobIO(ctx, a, args, os.Stdout, os.Stderr)
}

func runJobIO(ctx context.Context, a *app.App, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprint(stdout, jobUsage)
		return 0
	}
	switch args[0] {
	case "ls":
		if err := a.Check(ctx, "schedule", "list"); err != nil {
			fmt.Fprintln(stderr, app.DenyMessage(err))
			return 1
		}
		return jobList(stdout, stderr)
	case "add":
		if err := a.Check(ctx, "schedule", "add"); err != nil {
			fmt.Fprintln(stderr, app.DenyMessage(err))
			return 1
		}
		return jobAdd(args[1:], stdout, stderr)
	case "rm":
		if err := a.Check(ctx, "schedule", "remove"); err != nil {
			fmt.Fprintln(stderr, app.DenyMessage(err))
			return 1
		}
		if len(args) != 2 {
			fmt.Fprintln(stderr, "Usage: iugum job rm <name>")
			return 2
		}
		return jobRemove(args[1], stdout, stderr)
	case "run":
		if err := a.Check(ctx, "schedule", "run"); err != nil {
			fmt.Fprintln(stderr, app.DenyMessage(err))
			return 1
		}
		if len(args) != 2 {
			fmt.Fprintln(stderr, "Usage: iugum job run <name>")
			return 2
		}
		return jobRunNow(ctx, args[1], stderr)
	default:
		fmt.Fprintf(stderr, "job: unknown subcommand %s\n\n%s", args[0], jobUsage)
		return 2
	}
}

func jobList(stdout, stderr io.Writer) int {
	jobs, err := loadPersistedJobs()
	if err != nil {
		fmt.Fprintf(stderr, "job ls: %v\n", err)
		return 1
	}
	if len(jobs) == 0 {
		fmt.Fprintln(stdout, "no jobs")
		return 0
	}
	for _, j := range jobs {
		spec := j.Spec
		if spec == "" {
			spec = "@triggered"
		}
		kind := j.Kind
		if kind == "" {
			kind = "func"
		}
		fmt.Fprintf(stdout, "%s  %s  %s\n", j.Name, spec, kind)
	}
	return 0
}

func jobAdd(args []string, stdout, stderr io.Writer) int {
	spec, err := parseJobAdd(args)
	if err != nil {
		fmt.Fprintf(stderr, "job add: %v\n", err)
		return 2
	}
	jobs, err := loadPersistedJobs()
	if err != nil {
		fmt.Fprintf(stderr, "job add: %v\n", err)
		return 1
	}
	for _, j := range jobs {
		if j.Name == spec.Name {
			fmt.Fprintf(stderr, "job add: %s already exists\n", spec.Name)
			return 1
		}
	}
	jobs = append(jobs, spec)
	if err := writePersistedJobs(jobs); err != nil {
		fmt.Fprintf(stderr, "job add: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "job %s saved (%s). The running scheduler loads it within a second.\n", spec.Name, spec.Spec)
	return 0
}

func jobRemove(name string, stdout, stderr io.Writer) int {
	jobs, err := loadPersistedJobs()
	if err != nil {
		fmt.Fprintf(stderr, "job rm: %v\n", err)
		return 1
	}
	kept := jobs[:0]
	found := false
	for _, j := range jobs {
		if j.Name == name {
			found = true
			continue
		}
		kept = append(kept, j)
	}
	if !found {
		fmt.Fprintf(stderr, "job rm: %s not found\n", name)
		return 1
	}
	if err := writePersistedJobs(kept); err != nil {
		fmt.Fprintf(stderr, "job rm: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "job %s removed from jobs.yaml. Restart iugum up to drop it from the running scheduler.\n", name)
	return 0
}

func jobRunNow(ctx context.Context, name string, stderr io.Writer) int {
	jobs, err := loadPersistedJobs()
	if err != nil {
		fmt.Fprintf(stderr, "job run: %v\n", err)
		return 1
	}
	for _, j := range jobs {
		if j.Name != name {
			continue
		}
		if err := app.RunJobBody(ctx, j); err != nil {
			fmt.Fprintf(stderr, "job run: %v\n", err)
			return 1
		}
		return 0
	}
	fmt.Fprintf(stderr, "job run: %s not found\n", name)
	return 1
}

func parseJobAdd(args []string) (config.JobSpec, error) {
	var out config.JobSpec
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return out, fmt.Errorf("name is required")
	}
	out.Name = args[0]
	if !validJobName(out.Name) {
		return out, fmt.Errorf("name %q is not a single token [A-Za-z0-9._-]", out.Name)
	}
	args = args[1:]
	var every, spec, kind, prompt, url string
	command := []string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			command = append(command, args[i+1:]...)
			break
		}
		need := func(flag string) (string, error) {
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s needs a value", flag)
			}
			i++
			return args[i], nil
		}
		var err error
		switch a {
		case "--every":
			every, err = need(a)
		case "--spec":
			spec, err = need(a)
		case "--kind":
			kind, err = need(a)
		case "--prompt":
			prompt, err = need(a)
		case "--url":
			url, err = need(a)
		default:
			return out, fmt.Errorf("unknown flag %s", a)
		}
		if err != nil {
			return out, err
		}
	}
	switch {
	case spec != "":
		out.Spec = spec
	case every != "":
		out.Spec = "@every " + every
	default:
		out.Spec = "@triggered"
	}
	out.Prompt = prompt
	out.URL = url
	out.Command = command
	if kind == "" {
		if prompt != "" && len(command) == 0 {
			kind = "session"
		} else if url != "" && len(command) == 0 {
			kind = "http"
		} else {
			kind = "exec"
		}
	}
	out.Kind = kind
	switch kind {
	case "exec":
		if len(command) == 0 {
			return out, fmt.Errorf("kind exec needs -- <command>")
		}
	case "http":
		if url == "" {
			return out, fmt.Errorf("kind http needs --url")
		}
	case "session":
		if prompt == "" {
			return out, fmt.Errorf("kind session needs --prompt")
		}
	default:
		return out, fmt.Errorf("unknown kind %q (exec, http, session)", kind)
	}
	return out, nil
}

func validJobName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		ok := (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-' || r == '.'
		if !ok || (i == 0 && (r == '.' || r == '-' || r == '_')) {
			return false
		}
	}
	return true
}

func loadPersistedJobs() ([]config.JobSpec, error) {
	path := config.JobsFile()
	if path == "" {
		return nil, nil
	}
	return config.LoadJobsFile(path)
}

func writePersistedJobs(jobs []config.JobSpec) error {
	return config.WriteJobsFile(config.JobsFileForWrite(), jobs)
}
