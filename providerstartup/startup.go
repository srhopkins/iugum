// provider-startup supervises only its own authenticated loopback children.
package providerstartup

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type Settings struct {
	NativeRunScript    string   `json:"native_run_script,omitempty"`
	ProviderConfigFile string   `json:"provider_config_file,omitempty"`
	InstructionsFile   string   `json:"instructions_file,omitempty"`
	SkillsPath         string   `json:"skills_path,omitempty"`
	Mode               string   `json:"mode"`
	ExistingURL        string   `json:"existing_url,omitempty"`
	HostArgs           []string `json:"host_args"`
	HostConfig         string   `json:"host_config"`
	Root               string   `json:"root"`
	PasswordFile       string   `json:"password_file"`
	TokenFile          string   `json:"token_file"`
	ModelKeyFile       string   `json:"model_key_file"`
	ComparisonRoot     string   `json:"comparison_root"`
}

func privateRead(path string) ([]byte, error) {
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
		return nil, errors.New("credential must be private regular file")
	}
	if raw, ok := st.Sys().(*syscall.Stat_t); !ok || int(raw.Uid) != os.Getuid() {
		return nil, errors.New("credential must belong to service user")
	}
	b, e := io.ReadAll(io.LimitReader(f, 65537))
	if len(b) > 65536 {
		return nil, errors.New("credential too large")
	}
	if e != nil {
		return nil, e
	}
	if strings.TrimSpace(string(b)) == "" {
		return nil, errors.New("empty credential")
	}
	return b, nil
}
func password(path string) ([]byte, error) {
	b, e := privateRead(path)
	if !os.IsNotExist(e) {
		return b, e
	}
	raw := make([]byte, 48)
	if _, e = rand.Read(raw); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, 0600)
	if os.IsExist(e) {
		return privateRead(path)
	}
	if e != nil {
		return nil, e
	}
	_, e = f.WriteString(hex.EncodeToString(raw))
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e != nil {
		return nil, e
	}
	if closeErr != nil {
		return nil, closeErr
	}
	return privateRead(path)
}
func validate(s Settings) error {
	for _, p := range []string{s.ProviderConfigFile, s.InstructionsFile, s.SkillsPath, s.NativeRunScript} {
		if p != "" && !filepath.IsAbs(p) {
			return errors.New("optional startup references must be absolute")
		}
	}
	if s.Mode != "managed" && s.Mode != "existing" {
		return errors.New("mode must be managed or existing")
	}
	if s.Mode == "existing" {
		u, e := url.Parse(s.ExistingURL)
		if e != nil || u == nil || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "http" && u.Scheme != "https") {
			return errors.New("existing server requires explicit loopback HTTP URL")
		}
		ip := net.ParseIP(u.Hostname())
		if ip == nil || !ip.IsLoopback() {
			return errors.New("existing server must use loopback IP; remote server requires separate review")
		}
	}
	paths := []string{s.Root, s.PasswordFile, s.TokenFile, s.ComparisonRoot, s.HostConfig}
	if s.Mode == "managed" {
		paths = append(paths, s.ModelKeyFile)
	}
	for _, p := range paths {
		if !filepath.IsAbs(p) {
			return errors.New("all configured paths must be absolute")
		}
	}
	if len(s.HostArgs) < 4 || !filepath.IsAbs(s.HostArgs[0]) {
		return errors.New("exact existing host command required")
	}
	if len(s.HostArgs) < 4 || s.HostArgs[1] != "agent" || s.HostArgs[2] != "sessions" || s.HostArgs[3] != "serve" {
		return errors.New("exact host subcommand required")
	}
	allowed := map[string]bool{"--listen": true, "--data-dir": true, "--codex-bin": true, "--claude-bin": true, "--claude-actor": true, "--claude-account": true, "--claude-config-dir": true, "--claude-model": true, "--claude-policy": true, "--cursor-bin": true, "--native-url": true, "--native-cwd": true}
	values := map[string]string{}
	hostTools := false
	for i := 4; i < len(s.HostArgs); i++ {
		a := s.HostArgs[i]
		if a == "--host-tools" {
			if hostTools {
				return errors.New("duplicate host flag")
			}
			hostTools = true
			continue
		}
		if !allowed[a] {
			return errors.New("unsupported host flag; no feature expansion or alternate flag forms")
		}
		if _, ok := values[a]; ok {
			return errors.New("duplicate host flag")
		}
		if i+1 >= len(s.HostArgs) || strings.HasPrefix(s.HostArgs[i+1], "--") {
			return errors.New("missing host flag value")
		}
		i++
		values[a] = s.HostArgs[i]
	}
	if values["--listen"] != "127.0.0.1:3853" || !hostTools {
		return errors.New("host must preserve exact loopback editor endpoint")
	}
	if values["--data-dir"] != "/config/.local/share/iugum/chat-sessions" {
		return errors.New("host history directory must remain unchanged")
	}
	if values["--claude-policy"] != "/config/.config/iugum-chat/session-policy.csv" {
		return errors.New("existing provider policy must remain configured")
	}

	return nil
}
func envSet(env []string, key, value string) []string {
	out := []string{}
	for _, e := range env {
		if !strings.HasPrefix(e, key+"=") {
			out = append(out, e)
		}
	}
	return append(out, key+"="+value)
}
func config(s Settings) (string, error) {
	configPath := s.ProviderConfigFile
	if configPath == "" {
		configPath = filepath.Join(s.ComparisonRoot, "reviewed-editor-control-config.json")
	}
	b, e := os.ReadFile(configPath)
	if e != nil {
		return "", e
	}
	var cfg map[string]any
	if e = json.Unmarshal(b, &cfg); e != nil {
		return "", e
	}
	key, e := privateRead(s.ModelKeyFile)
	if e != nil {
		return "", e
	}
	token, e := privateRead(s.TokenFile)
	if e != nil {
		return "", e
	}
	provider, ok := cfg["provider"].(map[string]any)
	if !ok {
		return "", errors.New("missing provider")
	}
	route, ok := provider["litellm-homelab"].(map[string]any)
	if !ok {
		return "", errors.New("missing model route")
	}
	options, ok := route["options"].(map[string]any)
	if !ok {
		return "", errors.New("missing provider options")
	}
	options["apiKey"] = strings.TrimSpace(string(key))
	mcp, ok := cfg["mcp"].(map[string]any)
	if !ok {
		return "", errors.New("missing MCP config")
	}
	editor, ok := mcp["editor"].(map[string]any)
	if !ok {
		return "", errors.New("missing editor MCP")
	}
	if editor["url"] != "http://127.0.0.1:3853/api/host/mcp" {
		return "", errors.New("editor MCP must remain loopback")
	}
	editor["headers"] = map[string]string{"Authorization": "Bearer " + strings.TrimSpace(string(token))}
	cfg["model"] = "litellm-homelab/gpt-4o-mini"
	instructions := s.InstructionsFile
	if instructions == "" {
		instructions = filepath.Join(s.ComparisonRoot, "AGENTS.md")
	}
	cfg["instructions"] = []string{instructions}
	skills := s.SkillsPath
	if skills == "" {
		skills = filepath.Join(s.ComparisonRoot, ".agents/skills")
	}
	cfg["skills"] = map[string]any{"paths": []string{skills}}
	b, e = json.Marshal(cfg)
	return string(b), e
}

type child struct {
	cmd  *exec.Cmd
	done chan error
	log  *os.File
}

func start(args, env []string, cwd, logPath string) (*child, error) {
	f, e := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return nil, e
	}
	st, statErr := f.Stat()
	if statErr != nil || !st.Mode().IsRegular() {
		f.Close()
		return nil, errors.New("private log must be regular file")
	}
	if raw, ok := st.Sys().(*syscall.Stat_t); !ok || int(raw.Uid) != os.Getuid() {
		f.Close()
		return nil, errors.New("log ownership mismatch")
	}
	if e = f.Chmod(0600); e != nil {
		f.Close()
		return nil, e
	}
	c := exec.Command(args[0], args[1:]...)
	c.Env = env
	c.Dir = cwd
	c.Stdin = nil
	c.Stdout = f
	c.Stderr = f
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if e = c.Start(); e != nil {
		f.Close()
		return nil, e
	}
	v := &child{c, make(chan error, 1), f}
	go func() { v.done <- waitExited(c.Process.Pid); close(v.done) }()
	return v, nil
}
func stop(c *child) {
	if c == nil {
		return
	}
	// waitExited observes without reaping. The unreaped leader pins the group ID.
	_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGTERM)
	deadline := time.Now().Add(8 * time.Second)
	for groupAlive(c.cmd.Process.Pid) && time.Now().Before(deadline) {
		time.Sleep(25 * time.Millisecond)
	}
	if groupAlive(c.cmd.Process.Pid) {
		_ = syscall.Kill(-c.cmd.Process.Pid, syscall.SIGKILL)
		deadline = time.Now().Add(time.Second)
		for groupAlive(c.cmd.Process.Pid) && time.Now().Before(deadline) {
			time.Sleep(10 * time.Millisecond)
		}
	}
	<-c.done
	_ = c.cmd.Wait()
	_ = c.log.Close()
}

func ready(ctx context.Context, s Settings, old []byte, hostEndpoint string) error {
	client := &http.Client{Timeout: time.Second}
	for n := 0; n < 150; n++ {
		token, e := privateRead(s.TokenFile)
		if e == nil && string(token) != string(old) {
			req, _ := http.NewRequestWithContext(ctx, "GET", hostEndpoint+"/api/agents", nil)
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
			res, e := client.Do(req)
			if e == nil {
				io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
				res.Body.Close()
				if res.StatusCode == 200 {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return errors.New("host did not become ready with fresh MCP token")
}

type runtimeConfig struct {
	hostAddr, backendAddr string
	spawn                 func([]string, []string, string, string) (*child, error)
}

func Run(ctx context.Context, s Settings) error {
	return runWith(ctx, s, runtimeConfig{"127.0.0.1:3853", "127.0.0.1:4097", start})
}
func runWith(ctx context.Context, s Settings, runtime runtimeConfig) error {
	if e := validate(s); e != nil {
		return e
	}
	if e := os.MkdirAll(s.Root, 0700); e != nil {
		return e
	}
	st, e := os.Lstat(s.Root)
	if e != nil || !st.IsDir() || st.Mode().Perm()&0077 != 0 {
		return errors.New("provider root must be private directory")
	}
	if raw, ok := st.Sys().(*syscall.Stat_t); !ok || int(raw.Uid) != os.Getuid() {
		return errors.New("provider root ownership mismatch")
	}
	lock, e := os.OpenFile(filepath.Join(s.Root, "supervisor.lock"), os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW, 0600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		return errors.New("provider supervisor already running")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	// Never adopt or kill an unrelated listener, including a prior manual instance.
	ports := []string{runtime.hostAddr}
	if s.Mode == "managed" {
		ports = append(ports, runtime.backendAddr)
	}
	for _, addr := range ports {
		l, e := net.Listen("tcp", addr)
		if e != nil {
			return errors.New("provider port occupied; stop its verified owner before supervision")
		}
		l.Close()
	}
	var pw []byte
	if s.Mode == "managed" {
		pw, e = password(s.PasswordFile)
	} else {
		pw, e = privateRead(s.PasswordFile)
	}
	if e != nil {
		return e
	}
	old, _ := privateRead(s.TokenFile)
	endpoint := "http://" + runtime.backendAddr
	if s.Mode == "existing" {
		endpoint = s.ExistingURL
	}
	if s.Mode == "existing" {
		if e = verifyExisting(ctx, endpoint, pw); e != nil {
			return e
		}
	}
	args := append(append([]string{}, s.HostArgs...), "--opencode-url", endpoint, "--opencode-password-file", s.PasswordFile)
	env := envSet(os.Environ(), "IUGUM_CONFIG", s.HostConfig)
	host, e := runtime.spawn(args, env, s.ComparisonRoot, filepath.Join(s.Root, "host.log"))
	if e != nil {
		return e
	}
	defer stop(host)
	if e = ready(ctx, s, old, "http://"+runtime.hostAddr); e != nil {
		if ctx.Err() != nil {
			return nil
		}
		return e
	}
	if s.Mode == "existing" {
		fmt.Println("Host connected to existing loopback server; server lifecycle and credentials untouched")
		select {
		case <-ctx.Done():
			return nil
		case <-host.done:
			host = nil
			return errors.New("host exited")
		}
	}
	cfg, e := config(s)
	if e != nil {
		return e
	}
	backendEnv := envSet(envSet(os.Environ(), "OPENCODE_CONFIG_CONTENT", cfg), "OPENCODE_SERVER_PASSWORD", strings.TrimSpace(string(pw)))
	backend, e := runtime.spawn([]string{"opencode", "--pure", "serve", "--hostname", "127.0.0.1", "--port", strings.Split(runtime.backendAddr, ":")[1]}, backendEnv, s.ComparisonRoot, filepath.Join(s.Root, "server.log"))
	if e != nil {
		return e
	}
	defer stop(backend)
	if e = waitBackend(ctx, endpoint, pw, backend.done); e != nil {
		if ctx.Err() != nil {
			return nil
		}
		return e
	}
	fmt.Println("Loopback provider services started; private credential reused; fresh editor token loaded")
	select {
	case <-ctx.Done():
		return nil
	case <-host.done:
		host = nil
		return errors.New("host exited; backend stopped before supervised restart")
	case <-backend.done:
		backend = nil
		return errors.New("backend exited; host stopped before supervised restart")
	}
}

func verifyExisting(ctx context.Context, endpoint string, pw []byte) error {
	client := &http.Client{Timeout: 4 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}
	req, e := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(endpoint, "/")+"/global/health", nil)
	if e != nil {
		return errors.New("invalid existing endpoint")
	}
	req.SetBasicAuth("opencode", strings.TrimSpace(string(pw)))
	res, e := client.Do(req)
	if e != nil {
		return errors.New("existing server unavailable; verify address and credential reference")
	}
	defer res.Body.Close()
	if res.StatusCode == 401 || res.StatusCode == 403 {
		return errors.New("existing server rejected credential; saved credential not modified")
	}
	if res.StatusCode != 200 {
		return errors.New("existing server health failed; no server changes made")
	}
	var out struct {
		Healthy bool `json:"healthy"`
	}
	if json.NewDecoder(io.LimitReader(res.Body, 65536)).Decode(&out) != nil || !out.Healthy {
		return errors.New("existing server returned invalid health response")
	}
	protected, e := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(endpoint, "/")+"/provider", nil)
	if e != nil {
		return errors.New("invalid existing endpoint")
	}
	protected.SetBasicAuth("opencode", strings.TrimSpace(string(pw)))
	checked, e := client.Do(protected)
	if e != nil {
		return errors.New("existing protected provider endpoint unavailable")
	}
	checked.Body.Close()
	if checked.StatusCode == 401 || checked.StatusCode == 403 {
		return errors.New("existing server rejected credential; saved credential not modified")
	}
	if checked.StatusCode != 200 {
		return errors.New("existing provider catalog unavailable")
	}
	anonymous, e := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(endpoint, "/")+"/provider", nil)
	if e != nil {
		return errors.New("invalid existing endpoint")
	}
	probe, e := client.Do(anonymous)
	if e != nil {
		return errors.New("existing server authentication probe failed")
	}
	defer probe.Body.Close()
	if probe.StatusCode != 401 && probe.StatusCode != 403 {
		return errors.New("existing server does not require authentication; configuration unchanged")
	}
	return nil
}

func waitBackend(ctx context.Context, endpoint string, pw []byte, done <-chan error) error {
	deadline := time.NewTimer(15 * time.Second)
	defer deadline.Stop()
	for {
		if verifyExisting(ctx, endpoint, pw) == nil {
			return nil
		}
		select {
		case <-done:
			return errors.New("backend exited before ready")
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return errors.New("backend readiness timed out")
		case <-time.After(100 * time.Millisecond):
		}
	}
}
