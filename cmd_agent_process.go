package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/srhopkins/iugum/app"
	"gopkg.in/yaml.v3"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type agentProcess struct {
	Listen string `json:"listen"`
	Token  string `json:"token"`
	PID    int    `json:"pid"`
}

func processPaths(home string) (string, string) {
	return filepath.Join(home, "data", "runtime.json"), filepath.Join(home, "data", "runtime.lock")
}
func acquireAgentProcess(home, listen string) (agentProcess, func(), error) {
	file, lockPath := processPaths(home)
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		return agentProcess{}, nil, err
	}
	lock, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return agentProcess{}, nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return agentProcess{}, nil, fmt.Errorf("agent is already running")
	}
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		lock.Close()
		return agentProcess{}, nil, err
	}
	p := agentProcess{Listen: listen, Token: hex.EncodeToString(b), PID: os.Getpid()}
	data, _ := json.Marshal(p)
	if err = os.WriteFile(file, data, 0600); err != nil {
		lock.Close()
		return p, nil, err
	}
	return p, func() { os.Remove(file); syscall.Flock(int(lock.Fd()), syscall.LOCK_UN); lock.Close() }, nil
}
func runAgentProcess(ctx context.Context, a *app.App, verb string, args []string, out, errout io.Writer) int {
	fs := flag.NewFlagSet("agent "+verb, flag.ContinueOnError)
	fs.SetOutput(errout)
	home := fs.String("home", "", "agent home")
	open := fs.Bool("open", false, "runtime-only fully open policy override")
	if e := fs.Parse(args); e != nil {
		return 2
	}
	if *home == "" || fs.NArg() != 0 {
		fmt.Fprintln(errout, "Usage: iugum agent "+verb+" --home DIRECTORY")
		return 2
	}
	root, e := filepath.Abs(*home)
	if e != nil {
		fmt.Fprintln(errout, e)
		return 1
	}
	if e = a.Check(ctx, "agent", verb); e != nil {
		fmt.Fprintln(errout, e)
		return 1
	}
	fail := func(e error) int { fmt.Fprintln(errout, e); return 1 }
	path, _ := processPaths(root)
	if verb == "start" {
		if _, e = os.Stat(filepath.Join(root, "agent.yaml")); e != nil {
			return fail(e)
		}
		data, e := os.ReadFile(filepath.Join(root, "agent.yaml"))
		if e != nil {
			return fail(e)
		}
		var c struct {
			Listen string `yaml:"listen"`
		}
		if e = yaml.Unmarshal(data, &c); e != nil {
			return fail(e)
		}
		if c.Listen == "" {
			l, e := net.Listen("tcp", "127.0.0.1:0")
			if e != nil {
				return fail(e)
			}
			c.Listen = l.Addr().String()
			l.Close()
		}
		bin, e := os.Executable()
		if e != nil {
			return fail(e)
		}
		argv := []string{"agent", "run", "--home", root, "--listen", c.Listen}
		if *open {
			argv = append(argv, "--open")
			fmt.Fprintln(out, "OPEN MODE: policy bypass applies to this run only")
		}
		if e = os.MkdirAll(filepath.Join(root, "data"), 0700); e != nil {
			return fail(e)
		}
		log, e := os.OpenFile(filepath.Join(root, "data", "runtime.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if e != nil {
			return fail(e)
		}
		defer log.Close()
		cmd := exec.Command(bin, argv...)
		cmd.Stdout = log
		cmd.Stderr = log
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if e = cmd.Start(); e != nil {
			return fail(e)
		}
		exited := make(chan error, 1)
		go func() { exited <- cmd.Wait() }()
		for n := 0; n < 100; n++ {
			select {
			case e := <-exited:
				return fail(fmt.Errorf("agent exited before ready: %v; see %s", e, log.Name()))
			default:
			}
			if b, e := os.ReadFile(path); e == nil {
				var p agentProcess
				if json.Unmarshal(b, &p) == nil && p.PID == cmd.Process.Pid {
					client := http.Client{Timeout: time.Second}
					r, e := client.Get("http://" + p.Listen + "/api/status")
					if e == nil {
						r.Body.Close()
						if r.StatusCode == 200 {
							fmt.Fprintf(out, "Agent started at http://%s; log: %s\n", p.Listen, log.Name())
							return 0
						}
					}
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
		return fail(fmt.Errorf("agent has not reported ready; inspect %s (process may still be starting)", log.Name()))
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return fail(fmt.Errorf("agent is stopped or has no runtime record: %w", e))
	}
	var p agentProcess
	if e = json.Unmarshal(b, &p); e != nil {
		return fail(e)
	}
	client := http.Client{Timeout: 30 * time.Second}
	request := func(method, path, body string) ([]byte, error) {
		req, e := http.NewRequestWithContext(ctx, method, "http://"+p.Listen+path, strings.NewReader(body))
		if e != nil {
			return nil, e
		}
		req.Header.Set("Authorization", "Bearer "+p.Token)
		req.Header.Set("Content-Type", "application/json")
		r, e := client.Do(req)
		if e != nil {
			return nil, e
		}
		defer r.Body.Close()
		b, e := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if r.StatusCode >= 300 {
			return nil, fmt.Errorf("HTTP %d: %s", r.StatusCode, b)
		}
		return b, e
	}
	switch verb {
	case "stop":
		_, e = request("POST", "/api/runtime/stop", "")
		if e != nil {
			return fail(e)
		}
		fmt.Fprintln(out, "Stop requested.")
	case "status":
		b, e = request("GET", "/api/status", "")
		if e != nil {
			return fail(e)
		}
		fmt.Fprintln(out, string(b))
	case "attach":
		return attachAgentCLI(ctx, p.Listen, out, errout)
	}
	return 0
}

// attach is a client: EOF and /detach close the client, not the agent.
func attachAgentCLI(ctx context.Context, listen string, out, errout io.Writer) int {
	fmt.Fprintln(out, "Attached. Type /detach to leave the agent running.")
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	client := http.Client{Timeout: 6 * time.Minute}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "/detach" {
			return 0
		}
		if strings.TrimSpace(line) == "" {
			continue
		}
		b, _ := json.Marshal(map[string]string{"text": line})
		req, e := http.NewRequestWithContext(ctx, "POST", "http://"+listen+"/api/chat", bytes.NewReader(b))
		if e != nil {
			fmt.Fprintln(errout, e)
			return 1
		}
		req.Header.Set("Content-Type", "application/json")
		r, e := client.Do(req)
		if e != nil {
			fmt.Fprintln(errout, e)
			continue
		}
		body, e := io.ReadAll(io.LimitReader(r.Body, 2<<20))
		r.Body.Close()
		if e != nil {
			fmt.Fprintln(errout, e)
			continue
		}
		var reply struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(body, &reply) == nil && reply.Text != "" {
			fmt.Fprintln(out, reply.Text)
		} else {
			fmt.Fprintln(out, string(body))
		}
	}
	if e := scanner.Err(); e != nil {
		fmt.Fprintln(errout, e)
		return 1
	}
	return 0
}
