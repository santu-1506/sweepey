// Command sweepr cleans macOS and Windows machines: junk caches, complete
// app uninstalls with leftover hunting, project build artifacts, installer
// files, plus optional AI review of what is safe to delete.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"

	"github.com/santu-1506/sweepey/internal/fsutil"
	"github.com/santu-1506/sweepey/internal/safety"
	"github.com/santu-1506/sweepey/internal/ui"
)

var version = "0.1.0"

type opts struct {
	DryRun    bool
	Yes       bool
	AI        bool
	Permanent bool
	Trash     bool
	List      bool
	All       bool
	JSON      bool
	Quiet     bool
	Help      bool
	Top       int
	Args      []string
}

func parseArgs(args []string) (opts, error) {
	o := opts{Top: 20}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "--dry-run", "-n":
			o.DryRun = true
		case "--yes", "-y":
			o.Yes = true
		case "--ai":
			o.AI = true
		case "--permanent":
			o.Permanent = true
		case "--trash":
			o.Trash = true
		case "--list", "-l":
			o.List = true
		case "--all", "-a":
			o.All = true
		case "--json":
			o.JSON = true
		case "--quiet":
			o.Quiet = true
		case "--help", "-h":
			o.Help = true
		case "--top":
			if i+1 >= len(args) {
				return o, fmt.Errorf("--top needs a number")
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n < 1 {
				return o, fmt.Errorf("--top needs a positive number")
			}
			o.Top = n
		default:
			if strings.HasPrefix(a, "-") && len(a) > 1 {
				return o, fmt.Errorf("unknown flag %s (see sweepr help)", a)
			}
			o.Args = append(o.Args, a)
		}
	}
	return o, nil
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	args := os.Args[1:]
	cmd := ""
	if len(args) > 0 {
		cmd, args = args[0], args[1:]
	}
	o, err := parseArgs(args)
	if err != nil {
		ui.Fail("%v", err)
		os.Exit(2)
	}
	ui.AssumeYes = o.Yes
	if o.Help {
		usage()
		return
	}
	if err := run(ctx, cmd, o); err != nil {
		ui.Fail("%v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cmd string, o opts) error {
	switch cmd {
	case "":
		return menu(ctx)
	case "clean":
		return cmdClean(o)
	case "uninstall", "remove", "rm":
		return cmdUninstall(ctx, o)
	case "leftovers", "orphans":
		return cmdLeftovers(ctx, o)
	case "purge":
		return cmdPurge(o)
	case "installer", "installers":
		return cmdInstaller(o)
	case "analyze", "du":
		return cmdAnalyze(ctx, o)
	case "status":
		return cmdStatus(o)
	case "history":
		return cmdHistory(o)
	case "whitelist":
		return cmdWhitelist(o)
	case "ai":
		return cmdAI(ctx, o)
	case "version", "--version", "-v":
		fmt.Printf("sweepr %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
		return nil
	case "help", "--help", "-h":
		usage()
		return nil
	}
	return fmt.Errorf("unknown command %q (see sweepr help)", cmd)
}

func menu(ctx context.Context) error {
	if !ui.Interactive() {
		usage()
		return nil
	}
	items := []struct {
		key, label string
		cmd        string
	}{
		{"1", "Clean junk (caches, logs, temp, trash)", "clean"},
		{"2", "Uninstall an app completely", "uninstall"},
		{"3", "Find leftovers of deleted apps", "leftovers"},
		{"4", "Purge project build artifacts", "purge"},
		{"5", "Remove installer files", "installer"},
		{"6", "Analyze disk usage", "analyze"},
		{"7", "System status", "status"},
		{"8", "AI cleanup advisor", "ai"},
		{"9", "History", "history"},
	}
	for {
		fmt.Println()
		fmt.Println("  " + ui.Bold(ui.Cyan("sweepr")) + ui.Dim(" "+version+" · keep your machine clean"))
		fmt.Println()
		for _, it := range items {
			fmt.Printf("  %s  %s\n", ui.Bold(it.key), it.label)
		}
		fmt.Printf("  %s  %s\n", ui.Bold("q"), "Quit")
		choice := ui.Ask("\n  Choose:")
		if choice == "q" || choice == "" {
			return nil
		}
		for _, it := range items {
			if it.key == choice {
				o := opts{Top: 20}
				if it.cmd == "ai" {
					o.Args = []string{"advise"}
				}
				if err := run(ctx, it.cmd, o); err != nil {
					ui.Fail("%v", err)
				}
			}
		}
	}
}

func usage() {
	fmt.Printf(`%s %s — clean macOS and Windows, uninstall apps completely.

%s
  sweepr                         interactive menu
  sweepr clean                   caches, logs, temp files, browser & dev caches, trash
  sweepr uninstall [app]         remove an app and every file it left behind
  sweepr leftovers               find data from apps that are already deleted
  sweepr purge [dirs…]           node_modules, target/, .venv… in your projects
  sweepr installer               old .dmg / .pkg / .msi / setup.exe files
  sweepr analyze [dir]           explore what is using space
  sweepr status                  disk, memory and system overview
  sweepr history                 what sweepr removed, and when
  sweepr whitelist [add <path>]  paths sweepr must never touch
  sweepr ai advise               AI cleanup plan for this machine
  sweepr ai explain <path>       AI: what is this folder, is it safe to delete?

%s
  -n, --dry-run    show what would happen, change nothing
  -y, --yes        accept defaults without prompting
      --ai         let Claude review leftovers (uninstall, leftovers)
      --permanent  delete instead of moving to the Trash / Recycle Bin
      --trash      clean: move to the Trash instead of deleting
  -l, --list       uninstall: list installed apps
  -a, --all        clean: select every category
      --quiet      Windows uninstall: run the app's silent uninstaller
      --json       status, history: machine-readable output
      --top N      analyze: rows to show (default 20)

AI features need ANTHROPIC_API_KEY (or `+"`ant auth login`"+`). They send paths, names and
sizes only — never file contents. Set SWEEPR_AI_MODEL to change the model.
Operations log: %s
`, ui.Bold("sweepr"), version, ui.Bold("Commands"), ui.Bold("Flags"), fsutil.Tilde(safety.LogPath()))
}
