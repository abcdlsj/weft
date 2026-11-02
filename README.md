# weft

Weave many hosts into one subscription.

`weft` is a self-hosted control plane for [mihomo](https://github.com/MetaCubeX/mihomo) fleets. Every host runs a tiny agent that reports its live proxy inventory; weft weaves selected nodes from all of them into a single, revocable subscription URL and gives you a quiet web panel to manage it.

```
  host hk-01 ─┐
  host sg-02 ─┼─▶ weft ─▶ /sub/<token> ─▶ mihomo / Clash Verge / sing-box
  host us-03 ─┘
```

## Why

Running mihomo on several machines usually means several subscription URLs, drifting configs and no single place to see what is alive. weft keeps the hosts independent and does the joining on the server side.

## Features

- **Agent-based host management** — each host runs `weft agent`, authenticates once, and pushes its mihomo proxy inventory outbound. No inbound ports, no SSH credentials, works behind NAT.
- **One subscription URL per bundle** — pick nodes across hosts, get `/sub/<token>` in mihomo YAML, base64 or plain text. Tokens are revocable and track last use.
- **See which device pulls what** — the User-Agent becomes a client name (Clash Verge, sing-box, Shadowrocket, mihomo …), grouped per link with hit count, IP and last seen.
- **Rule templates** — keep the routing rules once, apply them to every bundle.
- **Panel** — live host status and latency, node picker, bundle editor, subscription management. Mobile first, installable as a PWA, tuned for Safari.
- **Demo mode** — `weft serve --demo` runs a fully populated instance with no agents.

![Overview](docs/screenshots/overview-light.jpg)

| Bundles | Subscriptions | Phone |
| --- | --- | --- |
| ![Bundles](docs/screenshots/bundles-light.jpg) | ![Subscriptions](docs/screenshots/subscriptions-light.jpg) | <img src="docs/screenshots/overview-phone.jpg" width="280" alt="Phone overview"> |

## Install

Grab the archive for your platform from [Releases](https://github.com/abcdlsj/weft/releases), or build it:

```bash
make build          # panel + binary, single file
./weft version
```

## Components

| Component | Where | What it does |
| --- | --- | --- |
| `weft serve` | `cmd/weft`, `internal/weft/server.go` | HTTP API, subscription endpoint, embedded panel, panel password and sessions |
| `weft agent` | `cmd/weft`, `internal/weft/agent.go`, `mihomo.go` | reads the local mihomo external-controller and reports proxies outbound |
| store | `internal/weft/store.go` | SQLite: hosts, nodes, bundles, subscriptions, reports, access log, settings |
| subscription builder | `internal/weft/sub.go` | template-driven mihomo YAML, base64 and plain link lists |
| device naming | `internal/weft/device.go` | User-Agent to client name, IP, optional `?tag=` override |
| panel | `web/` | React + TypeScript SPA, five screens, dark mode, PWA, self-hosted fonts and icons |
| demo fleet | `internal/weft/seed.go` | `--demo` seeds hosts, nodes, bundles and a live ticker |
| icon generator | `tools/icons` | renders the PWA icons from the brand mark |
| CI + release | `.github/workflows/` | gofmt/vet/test/build on every push, archives on every `v*` tag |

## Quick start

```bash
# server + panel, seeded with a demo fleet (panel password: weft-demo)
go run ./cmd/weft serve --demo --listen :8787

# or build the binary with the panel embedded
make build && ./weft serve --listen :8787 --password '<your password>'

# on each host with mihomo running
weft agent --server https://weft.example.com --token <agent-token> \
  --controller http://127.0.0.1:9090 --secret <mihomo-secret>
```

Open http://127.0.0.1:8787 and finish the setup wizard. The agent token is shown once, when you add the host.

## Status

Early. The design lives in [docs/DESIGN.md](docs/DESIGN.md).

## License

MIT
