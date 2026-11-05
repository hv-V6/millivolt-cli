package main

// mvctl — a standalone, zero-dependency Go CLI for millivolt.
//
// Build:   go build -o mvctl ./cmd/mvctl
// Run:     mvctl --url http://127.0.0.1:8471 status
// Test:    go test ./...
//
// This file is standalone-only. The shared core (model.go, client.go,
// format.go, core.go, core_test.go) is copied verbatim into the small upstream
// contribution at E:\dsh\millivolt-upstream-pr\cmd\mvctl.

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/hv-V6/mvctl/internal/config"
	"github.com/hv-V6/mvctl/internal/provision"
	"github.com/hv-V6/mvctl/internal/tui"
)

const versionString = "mvctl 0.1.0 (millivolt operator CLI)"

var globalBoolFlags = map[string]bool{
	"json": true, "no-color": true, "color": true, "yes": true,
	"verbose": true, "v": true, "help": true, "h": true, "version": true,
}

var globalValueFlags = map[string]bool{
	"url": true, "token": true, "timeout": true, "profile": true,
	"theme": true, "mode": true,
}

func main() {
	os.Exit(runWith(os.Args[1:], os.Stdout, os.Stderr))
}

func runWith(argv []string, stdout, stderr io.Writer) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	parsed, err := parseGlobal(argv)
	if err != nil {
		fmt.Fprintln(stderr, "mvctl: "+err.Error())
		return 2
	}
	g := parsed.global
	setColor(g.Color)
	tui.SetColor(g.Color)

	if parsed.showVersion {
		fmt.Fprintln(stdout, versionString)
		return 0
	}

	f, state, err := config.Load()
	if err != nil {
		fmt.Fprintln(stderr, "mvctl: "+err.Error())
		return 1
	}
	r := config.Resolve(f, state, parsed.flags)
	first := ""
	if len(parsed.args) > 0 {
		first = parsed.args[0]
	}
	localOnly := parsed.showHelp || parsed.showVersion || first == "help" ||
		first == "themes" || first == "profiles" || first == "" ||
		first == "install" || first == "uninstall"
	if !parsed.explicitURL && state.DeployDir == "" && !localOnly {
		r = autoDiscover(ctx, r)
	}
	g.BaseURL = r.BaseURL
	g.Token = r.Token
	g.Theme = r.Theme
	g.Mode = r.Mode
	g.Profile = r.Profile
	g.Args = parsed.args
	g.Flags = parsed.flags

	client := NewClient(g)
	cmds := append(CoreCommands(), ExtraCommands()...)
	cmds = append(cmds, LifecycleCommands()...)
	cmds = append(cmds, ShowcaseCommands()...)
	cmds = append(cmds, InstallCommands()...)

	// --help / -h must short-circuit before any command runs, regardless of
	// position, so `mvctl --help status` never hits the network.
	if parsed.showHelp {
		fmt.Fprintln(stdout, Help(cmds))
		return 0
	}

	result, err := Dispatch(cmds, Context{
		Ctx: ctx, G: g, C: client, Args: parsed.args, Flags: parsed.flags,
	})
	if err != nil {
		fmt.Fprintln(stderr, "mvctl: "+err.Error())
		if !g.JSON && result.Pretty != "" {
			fmt.Fprintln(stdout, result.Pretty)
		}
		if g.JSON {
			payload := map[string]any{"error": err.Error()}
			if result.Data != nil {
				payload["details"] = result.Data
			}
			fmt.Fprintln(stdout, toJSON(payload))
		}
		code := result.Code
		if code == 0 {
			code = 1
		}
		return code
	}
	if g.JSON && result.Data != nil {
		fmt.Fprintln(stdout, toJSON(result.Data))
	} else if result.Pretty != "" {
		fmt.Fprintln(stdout, result.Pretty)
	}
	return result.Code
}

type parsedArgs struct {
	global      Global
	flags       map[string]string
	args        []string
	showHelp    bool
	showVersion bool
	explicitURL bool
}

func parseGlobal(argv []string) (parsedArgs, error) {
	out := parsedArgs{
		global: Global{Color: true, Timeout: 10 * time.Second},
		flags:  map[string]string{},
	}
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		if a == "--" {
			// Everything after -- is positional, even if it looks like a flag.
			out.args = append(out.args, argv[i+1:]...)
			break
		}
		if strings.HasPrefix(a, "--") {
			body := a[2:]
			name, val := body, ""
			if eq := strings.Index(body, "="); eq >= 0 {
				name, val = body[:eq], body[eq+1:]
			}
			if globalBoolFlags[name] {
				// "--json=false" must actually disable the flag.
				enabled := boolFlagValue(val)
				switch name {
				case "json":
					out.global.JSON = enabled
				case "no-color":
					out.global.Color = !enabled
				case "color":
					out.global.Color = enabled
				case "yes":
					out.global.Yes = enabled
				case "verbose", "v":
					out.global.Verbose = enabled
				case "help", "h":
					out.showHelp = enabled
				case "version":
					out.showVersion = enabled
				}
				continue
			}
			if val == "" && i+1 < len(argv) && !strings.HasPrefix(argv[i+1], "-") {
				// Only global value flags can be mistaken for a command word;
				// per-command flags legitimately take any value (e.g. --name demo).
				if globalValueFlags[name] && knownCommand(argv[i+1]) {
					return out, fmt.Errorf("missing value for --%s", name)
				}
				val = argv[i+1]
				i++
			}
			if val == "" {
				val = "true"
			}
			if globalValueFlags[name] {
				applyGlobal(&out, name, val)
				out.flags[name] = val
				if name == "url" {
					out.explicitURL = true
				}
				continue
			}
			out.flags[name] = val
			continue
		}
		if strings.HasPrefix(a, "-") && len(a) > 1 {
			for _, letter := range a[1:] {
				switch letter {
				case 'j':
					out.global.JSON = true
				case 'h':
					out.showHelp = true
				case 'v':
					out.global.Verbose = true
				default:
					return out, fmt.Errorf("unknown short flag -%c", letter)
				}
			}
			continue
		}
		out.args = append(out.args, a)
	}
	if os.Getenv("MILLIVOLT_URL") != "" {
		out.explicitURL = true
	}
	if out.global.Color && (os.Getenv("NO_COLOR") != "" || os.Getenv("MVCTL_NO_COLOR") != "") {
		out.global.Color = false
	}
	return out, nil
}

// knownCommand prevents a value flag from swallowing the command word, e.g.
// `mvctl --url status` must not become BaseURL="status" with no command.
func knownCommand(word string) bool {
	set := map[string]bool{}
	collect := func(cmds []Command) {
		for _, c := range cmds {
			set[c.Name] = true
		}
	}
	collect(CoreCommands())
	collect(ExtraCommands())
	collect(LifecycleCommands())
	collect(ShowcaseCommands())
	collect(InstallCommands())
	return set[word] || word == "help"
}

// boolFlagValue treats an omitted value as true and recognises common false
// spellings, so --json=false and --no-color=0 behave as expected.
func boolFlagValue(val string) bool {
	if val == "" {
		return true
	}
	switch strings.ToLower(val) {
	case "false", "0", "no", "off":
		return false
	default:
		return true
	}
}

func applyGlobal(out *parsedArgs, name, val string) {
	switch name {
	case "url":
		out.global.BaseURL = val
	case "token":
		out.global.Token = val
	case "timeout":
		if d, err := time.ParseDuration(val); err == nil {
			out.global.Timeout = d
		} else if n, err := strconv.Atoi(val); err == nil {
			out.global.Timeout = time.Duration(n) * time.Second
		}
	case "profile":
		out.global.Profile = val
	case "theme":
		out.global.Theme = val
	case "mode":
		out.global.Mode = val
	}
}

// autoDiscover makes `mvctl status` work with zero configuration: find a
// running millivolt, adopt its port and read the token from its own .env.
func autoDiscover(ctx context.Context, r config.Resolved) config.Resolved {
	d := provision.Discover(ctx, provision.DiscoverOptions{ExcludePorts: []int{8472}})
	for _, c := range d.Candidates {
		if c.Port <= 0 {
			continue
		}
		r.BaseURL = fmt.Sprintf("http://127.0.0.1:%d", c.Port)
		r.Port = c.Port
		if c.DeployDir != "" {
			r.Dir = c.DeployDir
			if r.Token == "" {
				r.Token = provision.ReadEnv(c.DeployDir)["MILLIVOLT_OPERATOR_TOKEN"]
			}
		}
		return r
	}
	return r
}
