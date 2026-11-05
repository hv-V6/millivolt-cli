# Changelog

All notable changes to `mvctl` are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Documentation

- Rebuilt the README with a generated banner, an 8-palette theme matrix and a
  six-command gallery, all composed in Go by `tools/screenshots -assets-only`.
  Added badges, feature cards, a collapsible command reference and embedded
  screenshots.

## [0.1.1] - 2026-11-05

### Security

- `dashboard` now issues a one-time random URL secret and an `HttpOnly`,
  `SameSite=Strict` cookie. A `Host` header alone was not an auth boundary: any
  local process could previously reach the token-injecting proxy. Requests
  without the session cookie are refused, and the secret is dropped from the
  address bar via a redirect.
- The dashboard listener now reports a bind failure instead of silently
  claiming success, and the browser launcher is reaped instead of left as a
  zombie.

### Added

- `TestDashboardAuthGate` covers unauthenticated access, secret exchange,
  authenticated asset requests and forged cookies.

## [0.1.0] - 2026-11-05

First public release.

### Added

- **Standalone Go CLI** for millivolt: zero runtime dependencies, one binary,
  standard library only (the PNG screenshot tool is a separate Go module).
- **Auto-discovery** of a running deployment via Docker Compose labels,
  loopback `/healthz` probes and a bounded compose-file scan; the operator
  credential is read from the deployment's own `.env`.
- **View plane**: `status`, `overview`, `requests`, `query`, `prometheus`,
  `watch`, `chart`, `explore`, `cost`, `models`, `providers`, `trace`, `url`,
  `health`, `doctor`, `images`, `discover`, `logs`, `wait`.
- **Control plane**: `config get|set|reload|keys|diff`, `pause`, `throttle`,
  `quota`, `drain`, `debug`, `purge`, `backup`, `restore`, `restart`.
- **Deployment lifecycle**: `setup`, `adopt`, `start`, `stop`, `up`, `update`,
  `repair`, `revert`, `resume`, `serve`, `profiles`, `install`, `uninstall`.
- **Showcase**: `top` (live TUI), `themes`, `demo`, `bench`, `chat`, `tour`,
  `dashboard`. Eight millivolt palettes (`instrument`, `slate`, `midnight`,
  `nord`, `forest`, `rose`, `ember`, `contrast`) in `black` and `clear` modes.
- **Screenshots**: 74 PNGs of real command output plus a Go-only generator in
  `tools/screenshots`.
- Small upstream-contribution tree (`cmd/mvctl` + `docs/cli.md`) proposed as a
  human counterpart to millivolt's MCP tools.

### Security

- `--json` output redacts credential-looking keys inside typed structs, typed
  maps and nested values; error bodies are scrubbed of the operator token.
- `discover` emits `has_token` only, never a token value.
- Every state-changing command requires `--yes`; `purge` previews first and an
  unfiltered purge also requires `--all`.
- `--flag=false` / `=0` / `=no` / `=off` disables a boolean flag; `--` ends flag
  parsing; help short-circuits before any command runs.
- Backups stream to a temp file with `fsync`; an oversized response is an error
  rather than a silently truncated archive.
- Config/state writes are atomic, `fsync`ed, locked and 0600; corrupt files are
  reported rather than silently reset.
- Docker commands are argv-only with bounded output, timeouts and `WaitDelay`.

### Fixed during review

- Reflection-based redaction replacing a type switch that leaked typed secrets.
- Boolean `--flag=value` handling, `--` passthrough, `-v`, unknown short flags,
  missing value-flag detection and structured `--json` errors.
- `watch` printing literal `—` for errors/in-flight; sparkline resampling that
  produced false troughs.
- `drain --yes` widening the scope to all providers; `revert` erroring before
  its preview.
- Rollback override ignored by `update`; `Install` rotating an existing
  operator credential; foreign compose files under alternate names.
- `demo` error-rate overflow, loopback-only mock upstream under Docker, and
  64 KiB SSE scanner cap.
- TUI stale-frame clearing, stdin goroutine shutdown and non-finite sparkline
  panic; ANSI private-mode stripping.
- Flaky Docker-dependent tests (injected seams), corrupt-config swallowing,
  lock staleness, IPv6 `docker port` parsing, and weak token fallback.
