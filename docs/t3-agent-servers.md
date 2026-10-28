# T3 servers for the homelab agents

T3 is an optional web interface over the coding agents inside each container.
The server runs as `abc`. It keeps pairing authentication enabled.
Project folders and provider credentials stay in the existing `/config` volume.

| Environment | Browser URL | Persistent state |
|---|---|---|
| FFAI agent | https://ffai-agent-t3.local | `ffai-agent-config`, `/config/.t3` |
| iugum agent | https://iugum-agent-t3.local | `iugum-agent-config`, `/config/.t3` |

## New images

For Selkies, use `--build-arg T3=1 --build-arg T3_VERSION=0.1.0` with
`agentbox/Dockerfile`. For the service image, include `t3` in `WITH`.
`WITH=all` includes it. Both images install the complete official release
bundle, including the native terminal modules, after SHA-256 verification.

## Existing desktops without recreation

Copy `agentbox/runtime.sh` to the host's shared install folder as `69-t3.sh`.
Only containers with `/config/.t3/enabled` opt in. Create that marker as `abc`.
Run the hook as root to start it immediately in a dedicated
`t3-server` tmux session. Existing sessions are untouched.

The hook writes `/custom-services.d/t3-server` for s6 to supervise on later
container starts. On recreation it reinstalls the pinned release. Keep the
shared install.d mount and the `/config` volume. s6 discovers services before
init hooks run. If it has not discovered T3 yet (first install or recreation),
the hook uses tmux. If s6 already registered T3, the hook starts no second process.
The hook installs tmux if the base image does not include it.

On the homelab the hook lives at `/data/agentbox/install.d/69-t3.sh`.
`agentbox/t3.homelab.yaml` adds two forwarding containers on the existing `proxy`
network. They give Traefik and its mDNS helper the new hostnames without
recreating either desktop. Its deployed copy is `/data/agentbox/t3-compose.yaml`.
The forwarders resolve the agent container names, so recreation can change IPs.
No T3 port is published directly on the host.

The additional TLS certificate lives under `/home/steve/traefik/certs/t3-local.*`.
`/home/steve/traefik/dynamic/t3-tls.yml` loads it without replacing certificates
for other services. Certificate keys are never committed.

## Pairing and operation

Open the HTTPS address, then pair that browser once. Run `t3 pair` as `abc` inside
the target container to generate a fresh pairing token. Paste that token into
the HTTPS page. The printed URL can contain the internal container address.
The startup token is also in `/config/.t3/server.log`, readable only by its owner.
Do not paste tokens into project files or chat logs.

After a container restart, check `s6-svstat /run/service/custom-svc-t3-server`.
Before the first restart, check `tmux has-session -t t3-server` as `abc` instead.
Verify `curl -f https://ffai-agent-t3.local/` and the iugum URL from the Mac.
For a complete check, pair a browser and verify a terminal works; the terminal
requires the bundled native modules as well as the main executable.

## Disable

Stop the `t3-server` tmux session, or stop its s6 service if supervised there.
Remove only the opt-in marker and the T3 custom-service file to keep it disabled
at subsequent boots. Preserve `/config/.t3` to retain projects and conversations.
Remove the two forwarding containers with this Compose file if removing the routes.
