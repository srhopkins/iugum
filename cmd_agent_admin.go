package main

import (
	"bufio"
	"context"
	"encoding/xml"
	"flag"
	"fmt"
	"github.com/srhopkins/iugum/app"
	"github.com/srhopkins/iugum/policy"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func runAgentAdmin(ctx context.Context, a *app.App, verb string, args []string, out, errout io.Writer) int {
	fs := flag.NewFlagSet(verb, flag.ContinueOnError)
	fs.SetOutput(errout)
	home := fs.String("home", "", "agent home")
	source := fs.String("file", "", "proposed complete policy CSV")
	yes := fs.Bool("yes", false, "explicitly approve replacing the policy without prompting")
	if e := fs.Parse(args); e != nil {
		return 2
	}
	if *home == "" || fs.NArg() != 0 {
		fmt.Fprintln(errout, "--home DIRECTORY is required")
		return 2
	}
	if e := a.Check(ctx, "agent", verb); e != nil {
		fmt.Fprintln(errout, e)
		return 1
	}
	root, e := filepath.Abs(*home)
	if e != nil {
		fmt.Fprintln(errout, e)
		return 1
	}
	switch verb {
	case "supervisor":
		bin, e := os.Executable()
		if e != nil {
			fmt.Fprintln(errout, e)
			return 1
		}
		escape := func(s string) string { var b strings.Builder; xml.EscapeText(&b, []byte(s)); return b.String() }
		name := filepath.Base(root)
		fmt.Fprintf(out, `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>com.iugum.agent.%s</string>
<key>ProgramArguments</key><array><string>%s</string><string>agent</string><string>run</string><string>--home</string><string>%s</string></array>
<key>RunAtLoad</key><true/><key>KeepAlive</key><true/>
<key>StandardOutPath</key><string>%s</string><key>StandardErrorPath</key><string>%s</string>
</dict></plist>
`, escape(name), escape(bin), escape(root), escape(filepath.Join(root, "data", "runtime.log")), escape(filepath.Join(root, "data", "runtime.log")))
	case "policy-apply":
		if *source == "" {
			fmt.Fprintln(errout, "--file POLICY.csv is required")
			return 2
		}
		if _, e := policy.New("", *source); e != nil {
			fmt.Fprintln(errout, e)
			return 1
		}
		proposed, e := os.ReadFile(*source)
		if e != nil {
			fmt.Fprintln(errout, e)
			return 1
		}
		target := filepath.Join(root, "policy.csv")
		old, e := os.ReadFile(target)
		if e != nil {
			fmt.Fprintln(errout, e)
			return 1
		}
		fmt.Fprintf(out, "Policy replacement for %s\nCurrent:\n%s\nProposed:\n%s\n", root, old, proposed)
		if !*yes {
			fmt.Fprint(out, "Apply this permission change? [y/N] ")
			answer, _ := bufio.NewReader(os.Stdin).ReadString('\n')
			if strings.ToLower(strings.TrimSpace(answer)) != "y" {
				fmt.Fprintln(out, "Policy unchanged.")
				return 0
			}
		}
		temp, e := os.CreateTemp(root, ".policy-*")
		if e != nil {
			fmt.Fprintln(errout, e)
			return 1
		}
		defer os.Remove(temp.Name())
		temp.Chmod(0600)
		if _, e = temp.Write(proposed); e != nil {
			temp.Close()
			fmt.Fprintln(errout, e)
			return 1
		}
		if e = temp.Close(); e != nil {
			fmt.Fprintln(errout, e)
			return 1
		}
		if e = os.Rename(temp.Name(), target); e != nil {
			fmt.Fprintln(errout, e)
			return 1
		}
		fmt.Fprintln(out, "Policy replaced. Subsequent mediated calls use the new rules.")
	}
	return 0
}
