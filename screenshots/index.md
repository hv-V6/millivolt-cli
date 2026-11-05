# mvctl command verification screenshots

Every command was executed and its real terminal output captured as a PNG. Read-only commands used the live millivolt proxy; mutating commands used an in-process fake millivolt; Docker lifecycle commands used a harmless no-op scratch compose project that was torn down afterwards. No production state was changed.

| # | Command | Mode | Exit | Output | Screenshot | Note |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | `mvctl help` | local | 0 | 4585 B | [01-help.png](01-help.png) | command catalogue (PASS) |
| 2 | `mvctl --version` | local | 0 | 37 B | [02-version.png](02-version.png) | version (PASS) |
| 3 | `mvctl status` | live | 0 | 411 B | [03-status.png](03-status.png) | live health + headline metrics (PASS) |
| 4 | `mvctl --json status` | live | 0 | 1818 B | [04-status-json.png](04-status-json.png) | scripting output, redacted (PASS) |
| 5 | `mvctl overview` | live | 0 | 644 B | [05-overview.png](05-overview.png) | full KPI table (PASS) |
| 6 | `mvctl requests 10` | live | 0 | 1056 B | [06-requests.png](06-requests.png) | durable request log (PASS) |
| 7 | `mvctl query SELECT 1` | live | 0 | 23 B | [07-query.png](07-query.png) | bounded SQL (PASS) |
| 8 | `mvctl prometheus` | live | 0 | 1574 B | [08-prometheus.png](08-prometheus.png) | Prometheus exposition (PASS) |
| 9 | `mvctl watch --samples 3 --interval 1` | live | 0 | 279 B | [09-watch.png](09-watch.png) | live counter sampling (PASS) |
| 10 | `mvctl url` | live | 0 | 22 B | [10-url.png](10-url.png) | base URL (PASS) |
| 11 | `mvctl health` | live | 0 | 43 B | [11-health.png](11-health.png) | health gate (PASS) |
| 12 | `mvctl config get` | live | 0 | 4711 B | [12-config-get.png](12-config-get.png) | full live config (PASS) |
| 13 | `mvctl config keys listen` | live | 0 | 7 B | [13-config-keys.png](13-config-keys.png) | schema key search (PASS) |
| 14 | `mvctl config diff` | live | 0 | 518 B | [14-config-diff.png](14-config-diff.png) | config vs defaults (PASS) |
| 15 | `mvctl config set history_size=10000` | fake | 0 | 104 B | [15-config-set.png](15-config-set.png) | revision-checked write (fake) (PASS) |
| 16 | `mvctl config reload` | fake | 0 | 68 B | [16-config-reload.png](16-config-reload.png) | reload (fake) (PASS) |
| 17 | `mvctl pause` | live | 0 | 817 B | [17-pause-show.png](17-pause-show.png) | hold state (PASS) |
| 18 | `mvctl pause on` | fake | 0 | 88 B | [18-pause-on.png](18-pause-on.png) | add hold (fake) (PASS) |
| 19 | `mvctl throttle` | live | 0 | 469 B | [19-throttle-show.png](19-throttle-show.png) | provider limits (PASS) |
| 20 | `mvctl throttle set --provider openrouter --concurrency 8` | fake | 0 | 150 B | [20-throttle-set.png](20-throttle-set.png) | set limit (fake) (PASS) |
| 21 | `mvctl quota` | live | 0 | 168 B | [21-quota-show.png](21-quota-show.png) | storm/quota state (PASS) |
| 22 | `mvctl quota resume --provider openrouter` | fake | 0 | 72 B | [22-quota-resume.png](22-quota-resume.png) | resume gate (fake) (PASS) |
| 23 | `mvctl drain --all --yes` | fake | 0 | 50 B | [23-drain.png](23-drain.png) | drain queue (fake) (PASS) |
| 24 | `mvctl debug` | live | 0 | 3637 B | [24-debug-show.png](24-debug-show.png) | capture sessions (PASS) |
| 25 | `mvctl debug on` | fake | 0 | 77 B | [25-debug-on.png](25-debug-on.png) | start capture (fake) (PASS) |
| 26 | `mvctl purge --client mvctl-demo` | live | 0 | 173 B | [26-purge-preview.png](26-purge-preview.png) | preview only, no deletion (PASS) |
| 27 | `mvctl purge --client mvctl-demo --yes` | fake | 0 | 41 B | [27-purge-apply.png](27-purge-apply.png) | delete (fake) (PASS) |
| 28 | `mvctl backup --out BACKUP_FILE` | fake | 0 | 103 B | [28-backup.png](28-backup.png) | archive download (fake) (PASS) |
| 29 | `mvctl restore --file BACKUP_FILE` | fake | 0 | 149 B | [29-restore-inspect.png](29-restore-inspect.png) | inspect, no apply (PASS) |
| 30 | `mvctl restart` | live | 0 | 53 B | [30-restart-preview.png](30-restart-preview.png) | guarded preview (PASS) |
| 31 | `mvctl restart --yes` | fake | 0 | 69 B | [31-restart-apply.png](31-restart-apply.png) | restart (fake) (PASS) |
| 32 | `mvctl chart` | live | 0 | 1202 B | [32-chart.png](32-chart.png) | ASCII time series (PASS) |
| 33 | `mvctl explore --by provider --limit 6` | live | 0 | 1411 B | [33-explore.png](33-explore.png) | ranked breakdown (PASS) |
| 34 | `mvctl cost` | live | 0 | 1264 B | [34-cost.png](34-cost.png) | spend attribution (PASS) |
| 35 | `mvctl models` | live | 0 | 1378 B | [35-models.png](35-models.png) | model leaderboard (PASS) |
| 36 | `mvctl providers` | live | 0 | 1330 B | [36-providers.png](36-providers.png) | provider health board (PASS) |
| 37 | `mvctl trace a1` | fake | 0 | 134 B | [37-trace.png](37-trace.png) | single request detail (PASS) |
| 38 | `mvctl themes list` | local | 0 | 746 B | [38-themes-list.png](38-themes-list.png) | 8 palettes x 2 modes (PASS) |
| 39 | `mvctl wait --timeout 5s` | live | 0 | 39 B | [39-wait.png](39-wait.png) | block until healthy (PASS) |
| 40 | `mvctl profiles list` | local | 0 | 164 B | [40-profiles-list.png](40-profiles-list.png) | named deployments (PASS) |
| 41 | `mvctl profiles add --name demo --url http://127.0.0.1:8471` | local | 0 | 28 B | [41-profiles-add.png](41-profiles-add.png) | add profile (isolated config) (PASS) |
| 42 | `mvctl profiles use --name demo` | local | 0 | 31 B | [42-profiles-use.png](42-profiles-use.png) | set default profile (PASS) |
| 43 | `mvctl discover` | live | 0 | 429 B | [43-discover.png](43-discover.png) | Docker label discovery (PASS) |
| 44 | `mvctl doctor` | live | 0 | 848 B | [44-doctor.png](44-doctor.png) | 8-point diagnostic (PASS) |
| 45 | `mvctl images` | live | 0 | 233 B | [45-images.png](45-images.png) | image drift (PASS) |
| 46 | `mvctl logs --dir REAL_DIR --tail 15` | live | 0 | 16385 B | [46-logs.png](46-logs.png) | deployment logs (PASS) |
| 47 | `mvctl setup` | scratch | 0 | 79 B | [47-setup-preview.png](47-setup-preview.png) | guarded install preview (PASS) |
| 48 | `mvctl adopt --dir SCRATCH_DIR --port 8471` | scratch | 0 | 185 B | [48-adopt.png](48-adopt.png) | record deployment (isolated state) (PASS) |
| 49 | `mvctl stop --dir SCRATCH_DIR` | scratch | 0 | 46 B | [49-stop.png](49-stop.png) | stop (scratch project) (PASS) |
| 50 | `mvctl start --dir SCRATCH_DIR` | scratch | 0 | 17 B | [50-start.png](50-start.png) | start (scratch project) (PASS) |
| 51 | `mvctl up --dir SCRATCH_DIR` | scratch | 0 | 17 B | [51-up.png](51-up.png) | compose up (scratch) (PASS) |
| 52 | `mvctl update --dry-run --dir SCRATCH_DIR` | scratch | 0 | 28 B | [52-update.png](52-update.png) | dry-run update (PASS) |
| 53 | `mvctl revert` | scratch | 0 | 68 B | [53-revert-preview.png](53-revert-preview.png) | guarded rollback preview (PASS) |
| 54 | `mvctl resume` | scratch | 0 | 75 B | [54-resume-preview.png](54-resume-preview.png) | guarded resume preview (PASS) |
| 55 | `mvctl repair --dir SCRATCH_DIR` | scratch | 0 | 139 B | [55-repair.png](55-repair.png) | diagnose + fix (scratch) (PASS) |
| 56 | `mvctl serve` | scratch | 1 | 106 B | [56-serve.png](56-serve.png) | supervisor startup (PASS) |
| 57 | `mvctl top --no-tty --theme instrument --mode black` | live | 0 | 7040 B | [57-top-live.png](57-top-live.png) | TUI with real data (PASS) |
| 58 | `mvctl top --demo --no-tty --theme midnight --mode black` | local | 0 | 5252 B | [58-top-demo.png](58-top-demo.png) | TUI synthetic feed (PASS) |
| 59 | `mvctl tour --no-delay` | live | 0 | 6137 B | [59-tour.png](59-tour.png) | scripted walkthrough (PASS) |
| 60 | `mvctl dashboard --print` | live | 0 | 78 B | [60-dashboard-print.png](60-dashboard-print.png) | dashboard proxy URL (PASS) |
| 61 | `mvctl demo --requests 2 --yes --upstream http://127.0.0.1:1/v1` | fake | 0 | 348 B | [61-demo.png](61-demo.png) | showcase traffic (fake) (PASS) |
| 62 | `mvctl bench --requests 4 --concurrency 2 --yes --upstream http://127.0.0.1:1/v1` | fake | 0 | 353 B | [62-bench.png](62-bench.png) | latency percentiles (fake) (PASS) |
| 63 | `mvctl chat hello from mvctl --upstream http://127.0.0.1:1/v1` | fake | 0 | 44 B | [63-chat.png](63-chat.png) | one-shot inference (fake) (PASS) |
| 64 | `mvctl install` | local | 0 | 121 B | [64-install.png](64-install.png) | self-install preview (PASS) |
| 65 | `mvctl uninstall` | local | 0 | 87 B | [65-uninstall.png](65-uninstall.png) | uninstall preview (PASS) |
| 66 | `mvctl themes preview --theme instrument --mode black` | local | 0 | 1239 B | [66-theme-instrument.png](66-theme-instrument.png) | palette instrument / black (PASS) |
| 67 | `mvctl themes preview --theme slate --mode black` | local | 0 | 1219 B | [67-theme-slate.png](67-theme-slate.png) | palette slate / black (PASS) |
| 68 | `mvctl themes preview --theme midnight --mode black` | local | 0 | 1217 B | [68-theme-midnight.png](68-theme-midnight.png) | palette midnight / black (PASS) |
| 69 | `mvctl themes preview --theme nord --mode black` | local | 0 | 1216 B | [69-theme-nord.png](69-theme-nord.png) | palette nord / black (PASS) |
| 70 | `mvctl themes preview --theme forest --mode black` | local | 0 | 1208 B | [70-theme-forest.png](70-theme-forest.png) | palette forest / black (PASS) |
| 71 | `mvctl themes preview --theme rose --mode black` | local | 0 | 1215 B | [71-theme-rose.png](71-theme-rose.png) | palette rose / black (PASS) |
| 72 | `mvctl themes preview --theme ember --mode black` | local | 0 | 1190 B | [72-theme-ember.png](72-theme-ember.png) | palette ember / black (PASS) |
| 73 | `mvctl themes preview --theme contrast --mode black` | local | 0 | 1211 B | [73-theme-contrast.png](73-theme-contrast.png) | palette contrast / black (PASS) |
| 74 | `mvctl themes preview --theme rose --mode clear` | local | 0 | 1190 B | [74-theme-clear.png](74-theme-clear.png) | palette rose / clear (PASS) |
