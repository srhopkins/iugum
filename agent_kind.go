package main

import "fmt"

// chromeCLISelkies is the CHROME_CLI value from
// homelab/agentbox/compose-local.yaml, minus the trailing URL, which is
// specific to that one Mac instance and not part of the kind preset.
const chromeCLISelkies = "--remote-debugging-port=9222 --remote-debugging-address=127.0.0.1 --user-data-dir=/config/.chrome-profile --disable-dev-shm-usage --force-device-scale-factor=1.25"

// kindNames lists the built-in --kind presets kindPreset accepts.
func kindNames() []string {
	return []string{"selkies"}
}

// kindPreset returns the AgentFile for a built-in agent kind, adjusted for
// the active Docker context's routing (Traefik on the homelab tower, or
// caddy-docker-proxy on the Mac).
func kindPreset(kind, dockerContext, name string) (AgentFile, error) {
	switch kind {
	case "selkies":
		return selkiesPreset(dockerContext, name), nil
	default:
		return AgentFile{}, fmt.Errorf("unknown agent kind %q (known kinds: %v)", kind, kindNames())
	}
}

func selkiesPreset(dockerContext, name string) AgentFile {
	af := AgentFile{
		Name:    name,
		Image:   "agentbox:latest",
		Kind:    "selkies",
		User:    "",
		ShmSize: "1g",
		Volumes: []string{name + "-config:/config"},
		Env: []string{
			"PUID=1000",
			"PGID=1000",
			"TZ=America/Los_Angeles",
			"CHROME_CLI=" + chromeCLISelkies,
		},
		Mem:  "5g",
		Cpus: "3",
		Startup: AgentStartup{
			Restart: defaultRestart,
		},
	}

	switch dockerContext {
	case "docker-mac", "desktop-linux", "default":
		af.Network.External = "agentbox"
		af.Labels = []string{
			"caddy_0=http://" + name + ".localhost",
			"caddy_0.reverse_proxy={{upstreams 3000}}",
			"caddy_1=http://" + name + "-code.localhost",
			"caddy_1.reverse_proxy={{upstreams 8080}}",
		}
		af.Ports = []string{
			"127.0.0.1:3010:3000",
			"127.0.0.1:3011:3001",
			"127.0.0.1:8090:8080",
		}
	default: // "homelab", "", or any unrecognised value
		af.Network.External = "proxy"
		af.Labels = []string{
			"traefik.enable=true",
			"traefik.tcp.routers." + name + ".rule=HostSNI(`" + name + ".local`)",
			"traefik.tcp.routers." + name + ".entrypoints=websecure",
			"traefik.tcp.routers." + name + ".tls.passthrough=true",
			"traefik.tcp.routers." + name + ".service=" + name,
			"traefik.tcp.services." + name + ".loadbalancer.server.port=3001",
			"traefik.http.middlewares." + name + "-ssl.redirectscheme.scheme=https",
			"traefik.http.routers." + name + "-http.rule=Host(`" + name + ".local`)",
			"traefik.http.routers." + name + "-http.entrypoints=web",
			"traefik.http.routers." + name + "-http.middlewares=" + name + "-ssl",
			"traefik.http.routers." + name + "-http.service=" + name + "-http",
			"traefik.http.services." + name + "-http.loadbalancer.server.port=3000",
			"traefik.http.routers." + name + "-code.rule=Host(`" + name + "-code.local`)",
			"traefik.http.routers." + name + "-code.entrypoints=web,websecure",
			"traefik.http.routers." + name + "-code.tls=true",
			"traefik.http.routers." + name + "-code.service=" + name + "-code",
			"traefik.http.services." + name + "-code.loadbalancer.server.port=8080",
		}
	}

	return af
}
