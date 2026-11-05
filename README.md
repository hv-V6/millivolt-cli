# mvctl

> Control, inspect and show off a [millivolt](https://github.com/LLM-4-People/millivolt)
> inference proxy from a single Go binary.

`mvctl` is a standalone, zero-dependency CLI for millivolt. It reads live AI
traffic, drives the operator plane, manages the Docker deployment, and renders a
full-screen terminal dashboard using millivolt's own theme palettes.

It has **no runtime dependencies**: no Node, no Python, no shell scripts, no
package manager. Just one statically linked Go binary.

- **Control** — config, pause holds, provider throttles, quota gates, queue
  draining, debug capture, purge, backup/restore, restart.
- **View** — KPIs, retained request log, SQL queries, charts, cost attribution,
  model/provider leaderboards, Prometheus exposition, health diagnostics.
- **Show off** — a live TUI, 8 upstream themes × 2 modes, a traffic generator,
  a load benchmark, a scripted tour and a token-free dashboard proxy.

A deliberately small subset of this tool (`cmd/mvctl`, HTTP-only, no Docker, no
TUI) is also proposed upstream as a human counterpart to millivolt's MCP tools:
[LLM-4-People/millivolt PR](https://github.com/LLM-4-People/millivolt/pulls).

---

## Install

### Prebuilt binaries

Download the archive for your platform from the
[Releases](https://github.com/hv-V6/millivolt-cli/releases) page, extract it,
and put `mvctl` (or `mvctl.exe`) on your `PATH`.

| Platform | Asset |
| --- | --- |
| Windows x86-64 | `mvctl-windows-amd64.zip` |
| Linux x86-64 | `mvctl-linux-amd64.tar.gz` |
| Linux ARM64 | `mvctl-linux-arm64.tar.gz` |
| macOS x86-64 | `mvctl-darwin-amd64.tar.gz` |
| macOS ARM64 | `mvctl-darwin-arm64.tar.gz` |

### From source

Go 1.27.2 or newer:

```sh
git clone https://github.com/hv-V6/millivolt-cli.git
cd millivolt-cli
go build -o mvctl ./cmd/mvctl
```

### Self-install

Once you have the binary, it can install itself onto your `PATH`:

```sh
mvctl install          # preview: shows the target and PATH status
mvctl install --yes    # copies itself into ~/.local/bin (or --dir PATH)
mvctl status
mvctl uninstall --yes  # remove the installed copy
```

`install` reports whether the target directory is on your `PATH` and prints the
directory to add if it is not. It is pure Go — no shim, no `.cmd` wrapper, no
shell script. Re-running it is idempotent (detected by content hash).

---

## Quickstart

`mvctl` auto-discovers a running millivolt on first use: Docker Compose labels,
then a loopback `/healthz` probe, then a bounded compose-file scan. It reads the
operator credential from the deployment's own `.env`. **Nothing needs configuring
by hand.**

```sh
mvctl status                 # health, URL, version, headline metrics
mvctl overview               # full KPI / storage / operator state
mvctl requests 20            # recent proxied requests
mvctl top                    # live full-screen dashboard
```

Point it at a specific instance when needed:

```sh
mvctl --url http://127.0.0.1:8471 status
export MILLIVOLT_OPERATOR_TOKEN=...   # or --token / a profile / deployment .env
```

Everything supports `--json` for scripting and honours `--no-color` / `NO_COLOR`.

---

## Commands

Run `mvctl help` for the live list.

| Group | Commands |
| --- | --- |
| **View** | `status`, `overview`, `requests [N]`, `query "SELECT ..."`, `prometheus`, `watch`, `url`, `health`, `chart`, `explore`, `cost`, `models`, `providers`, `trace <id>` |
| **Control** | `config get\|set\|reload\|keys\|diff`, `pause [on\|off]`, `throttle set\|clear`, `quota [resume]`, `drain`, `debug [on\|off]`, `purge`, `backup`, `restore`, `restart` |
| **Manage** | `discover`, `setup`, `adopt`, `start`, `stop`, `up`, `update`, `repair`, `revert`, `resume`, `logs`, `doctor`, `images`, `serve`, `profiles`, `wait`, `install`, `uninstall` |
| **Show** | `top`, `themes list\|preview`, `demo`, `bench`, `chat`, `tour`, `dashboard` |

Global flags: `--json`, `--no-color` / `--color`, `--yes`, `--url`, `--token`,
`--timeout`, `--profile`, `--theme`, `--mode`, `--dir`, `--port`, `--verbose`,
`--version`, `--help`. `--` ends flag parsing; `--flag=false` (or `=0`, `=no`,
`=off`) disables a boolean flag.

### Examples

```sh
mvctl chart --metric cost
mvctl explore --by provider --limit 8
mvctl cost --by model
mvctl doctor                         # 8-point read-only diagnostic
mvctl --json status | jq .kpi.cost

mvctl config keys listen
mvctl config set history_size=10000 --yes
mvctl pause on --yes
mvctl throttle set --provider openrouter --concurrency 8 --yes
mvctl purge --client mvctl-demo --yes
mvctl backup --out backup.mvb
mvctl restore --file backup.mvb      # inspect only; add --yes to apply
```

### Safety

Every state-changing command is explicit: without `--yes` it prints a preview
and performs no request. This covers `config set|reload`, `pause on|off`,
`throttle set|clear`, `quota resume`, `drain`, `debug on|off`, `restart`,
`setup`, `revert`, `resume`, `update`, `start`, `stop`, `up`, `repair`, `adopt`,
`profiles add|remove|use`, `demo`, `bench` and `chat`.

Additional protections:

- `purge` previews with `/admin/purge/count`; an unfiltered purge also requires
  `--all`.
- `backup` refuses to overwrite an existing file without `--overwrite=true` and
  streams large archives to a temp file (a truncated archive is an error, not a
  corrupt "success").
- `config set` uses the server's `revision` for optimistic concurrency (409 on
  a stale write).
- `--json` redacts credential-looking fields **inside typed structs and typed
  maps**, and error bodies are scrubbed of the operator token.
- `discover` reports only `has_token`, never a token value.
- Docker is invoked with argv arrays (never a shell), bounded by timeouts and
  capped output, with `WaitDelay` so a hung child cannot block forever.

---

## Themes

`top` ships millivolt's own palette registry as its baseline, derived from
`internal/web/static/css/themes.css` and `static/js/theme.js`:

| id | hint | id | hint |
| --- | --- | --- | --- |
| `instrument` (default) | Warm graphite, blue and violet | `forest` | Moss, emerald and lime |
| `slate` | Cool stone, lilac and sand | `rose` | Plum, pink and lilac |
| `midnight` | Deep navy, sky and periwinkle | `ember` | Charcoal, orange and orchid |
| `nord` | Arctic slate, frost and heather | `contrast` | Maximum separation |

Each palette has a `black` and a `clear` mode. The TUI uses the real surface,
accent, ink and semantic status tokens for the header, KPI tiles, sparkline,
provider bars, request table and badges.

```sh
mvctl themes list
mvctl themes preview --theme nord --mode clear
mvctl top --theme midnight --mode black
mvctl --theme instrument --mode clear top --demo
```

`top --demo` runs a synthetic feed that writes **nothing** to millivolt, so it
is safe for screenshots and recordings. When stdout is not a TTY, `top` prints
one static frame instead of entering the alternate screen.

---

## Showing it off

- **`mvctl top`** — live dashboard: KPI tiles, a requests/sample sparkline,
  provider bars, a colour-coded request log, badges, and keybindings
  (`q` quit, `p` pause, `d` debug, `r` refresh, `t` theme, `m` mode, `?` help).
- **`mvctl demo --yes`** — starts an in-process OpenAI-compatible mock upstream
  and sends labelled traffic (`X-Proxy-Client: mvctl-demo`) through millivolt.
  Clean up with `mvctl purge --client mvctl-demo --yes`.
- **`mvctl bench --yes`** — concurrent load test with p50/p95/p99 latency.
- **`mvctl chat "hello" --upstream URL --yes`** — one-shot request through the
  proxy with the correct `X-Proxy-*` headers.
- **`mvctl tour`** — scripted status → overview → requests → chart → explore
  walkthrough for a recording.
- **`mvctl dashboard`** — loopback proxy that injects the operator credential
  and strips frame-blocking headers, then opens the real millivolt dashboard.
  No token pasting.

If the mock upstream is unreachable from inside Docker, pass
`--upstream http://host.docker.internal:<port>/v1`, or use `top --demo`.

### Verification screenshots

[`screenshots/index.md`](screenshots/index.md) contains a PNG of every command's
real output (74 images), generated by the pure-Go tool in
[`tools/screenshots`](tools/screenshots). Read-only commands were captured
against a live proxy; mutating commands against an in-process fake millivolt;
Docker lifecycle commands against a throwaway no-op compose project.

---

## Multiple deployments

```sh
mvctl profiles add --name prod --url http://127.0.0.1:8471 --dir ~/millivolt --yes
mvctl profiles use --name prod --yes
mvctl --profile prod status
```

Config and state live in the per-user config directory (`config.json`,
`state.json`) with atomic `fsync` writes and a lock file. Named profiles keep
their own URL, directory, token, port and theme. Precedence is
**flags → environment → profile → state → deployment `.env` → defaults**.

Environment variables: `MILLIVOLT_URL`, `MILLIVOLT_OPERATOR_TOKEN`,
`MILLIVOLT_PORT`, `MVCTL_PORT`, `MVCTL_PROFILE`, `MVCTL_THEME`, `MVCTL_MODE`,
`NO_COLOR`, `MVCTL_NO_COLOR`.

> On Windows, Unix permission bits are ignored by NTFS; `config.json`,
> `state.json` and a deployment `.env` can contain the operator token, so keep
> the containing directory private to your user account.

---

## Architecture

```
cmd/mvctl/            CLI entry, command registry, shared operator core
  main.go             flags, dispatch, auto-discovery
  model.go            tolerant wire decoders
  client.go           Bearer-auth HTTP client (capped + streaming download)
  format.go           tables, sparklines, ANSI, credential redaction
  core.go             view + control commands (shared with upstream PR)
  extra.go            chart / explore / cost / models / providers / trace
  lifecycle.go        Docker deployment lifecycle
  showcase.go         TUI, demo, bench, chat, tour, dashboard
  install.go          self-install / uninstall
internal/
  config/             config, state, profiles, locked atomic writes
  docker/             argv-only Docker client, bounded output
  provision/          discovery, install, update, rollback, repair
  themes/             8 millivolt palettes x 2 modes
  tui/                ANSI renderer, dashboard, raw terminal (build-tagged)
  demo/               mock upstream and traffic generator
tools/screenshots/    Go-only PNG screenshot generator
```

The five files `model.go`, `client.go`, `format.go`, `core.go`, `core_test.go`
are **copied verbatim** into the upstream contribution. The standalone build
adds everything else on top of that shared core.

---

## Development

```sh
go test ./...            # unit + in-process HTTP smoke tests
go test -race ./...      # race detector
go vet ./...
gofmt -l cmd internal
go build ./cmd/mvctl
```

No test needs a live Docker daemon or network access. Regenerate the
verification screenshots with `tools/screenshots` (see its `go.mod`).

---

## Related

- [millivolt](https://github.com/LLM-4-People/millivolt) — the inference proxy.
- [bb-plugin-millivolt](https://github.com/hv-V6/bb-plugin-millivolt) — the bb
  sidebar plugin whose provisioning logic this tool ports to Go.
- Upstream proposal: `cmd/mvctl` — a thin human client over millivolt's
  existing `/admin/*` and `/metrics/*` operator plane, sibling to the 22 MCP
  tools. No new endpoints, no new dependencies, no container management.

## License

MIT — see [LICENSE](LICENSE). Palette values are derived from millivolt's
MIT-licensed `themes.css`.
