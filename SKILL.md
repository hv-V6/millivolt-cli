---
name: mvctl
description: Control, inspect and demonstrate a millivolt inference proxy from the terminal with the standalone mvctl Go CLI. Use when the user asks about AI traffic, tokens, cost, errors, proxy health, provider limits, pause holds, or wants a live dashboard or demo.
---

# mvctl

`mvctl` is a standalone Go CLI for the millivolt inference proxy. It needs no bb
and no configuration: it discovers a running millivolt and reads the operator
credential from the deployment's `.env`.

## Start here

```sh
mvctl status                 # health + headline metrics
mvctl status --json          # machine-readable
mvctl overview               # full KPI / storage / operator state
mvctl requests 20            # retained request log
mvctl doctor                 # read-only full diagnostic
```

Point at a specific deployment with `--url http://127.0.0.1:8471` or
`MILLIVOLT_URL`. The token comes from `MILLIVOLT_OPERATOR_TOKEN`; it is never
printed and `--json` redacts secrets.

## Reading traffic

- `mvctl chart`, `mvctl explore --by provider|model|client|status|error`,
  `mvctl cost`, `mvctl models`, `mvctl providers` for analysis.
- `mvctl trace <request-id>` for one request's full record.
- `mvctl query "SELECT ..."` for bounded SQL over durable history.
- `mvctl watch` for a live sparkline; `mvctl prometheus` for exposition.
- `mvctl top` for the full-screen live TUI (add `top --demo` for a synthetic
  feed that writes nothing to millivolt).

## Control

- `mvctl config get|set KEY=VALUE|reload|keys|diff` (revision-checked).
- `mvctl pause [on|off]`, `mvctl throttle set|clear --provider X`,
  `mvctl quota resume --provider X`, `mvctl drain --all`, `mvctl debug [on|off]`.
- `mvctl purge` previews first; `mvctl restore` inspects first;
  `mvctl restart`, `mvctl purge`, `mvctl setup`, `mvctl demo`, `mvctl bench`
  and `mvctl revert` require `--yes`.

## Deployment

`mvctl discover` before `mvctl setup`: adopt an existing install
(`mvctl adopt --dir PATH --port N`) rather than creating a second copy.
`start|stop|up|update|repair|revert|resume|logs|images|serve|wait` manage the
Docker deployment. `stop` is sticky until `start`.

## Showing it off

- `mvctl themes list` and `mvctl themes preview --theme nord --mode clear`:
  the eight millivolt palettes in both black and clear modes.
- `mvctl top --theme midnight --mode black`: live dashboard with the same
  palette tokens the millivolt dashboard uses.
- `mvctl demo --yes` / `mvctl bench --yes` / `mvctl tour` / `mvctl dashboard`.

## Rules

- Never print or log the operator token.
- Never run destructive commands (`purge`, `restart`, `restore` without
  inspect, `revert`) without explicit user confirmation; pass `--yes` only then.
- Prefer `--json` when a script or agent consumes the output.
- Demo traffic is labeled `mvctl-demo`; clean it up with
  `mvctl purge --client mvctl-demo --yes`.
- The listener is loopback only; do not expose it.
