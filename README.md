# sweepr

A cleaner for **macOS and Windows** in one binary, inspired by [Mole](https://github.com/tw93/Mole).
It clears junk, uninstalls apps **completely** (it hunts down every file, launch agent and registry key
an app leaves behind), and can ask Claude to judge what is safe to delete.

```
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
```

Every destructive command supports `--dry-run`. Run it first.

## Interface

In a terminal, sweepr runs as a full-screen app (built with [Bubble Tea](https://github.com/charmbracelet/bubbletea)):

- **Home screen:** animated gradient logo, action menu (`↑↓`, `enter`, or `1`–`9`) and a live system panel with disk, memory, CPU sparkline and a health score.
- **Pickers:** for clean, uninstall, leftovers, purge and installers. `space` toggles, `a`/`n`/`i` select all/none/invert,
  `/` fuzzy-filters, `s` sorts. Each row has size bars and confidence badges, there is a detail pane for the
  highlighted row, and `✦AI✓`/`✦AI✗` marks Claude's verdict.
- **`sweepr status`:** live dashboard with CPU and memory charts, per-core load, disks, network rates and the top processes (`s` sorts by CPU or memory).
- **`sweepr analyze`:** disk explorer. `→` opens a folder, `←` goes back, `t` moves to the Trash, `r` reveals in Finder/Explorer, `e` asks Claude what a folder is.
- Spinners with progress bars, confirm dialogs (red for permanent deletes) and an animated result card.

Pipes, scripts, `--yes`, `--json` and `--plain` (or `SWEEPR_PLAIN=1`) get plain line-based output instead.

## Install

```sh
go install github.com/santu-1506/sweepey/cmd/sweepr@latest
```

## Build

Requires Go 1.26+ (see go.mod).

```sh
make build          # → bin/sweepr for this machine
make dist           # → dist/ binaries for macOS (arm64/amd64), Windows (amd64/arm64), Linux
make test
```

On Windows, copy `sweepr-windows-amd64.exe` anywhere on your `PATH` and run it from PowerShell or
Windows Terminal. Run it from an Administrator terminal to clean system-wide locations.

## Complete uninstall: how leftovers are found

`sweepr uninstall Slack --dry-run` builds a set of identifiers for the app and scans every place
apps keep data:

| | macOS | Windows |
|---|---|---|
| **Identifiers** | bundle ID, bundle name, executable, display name, vendor (from the bundle ID) | display name, executable, publisher, install folder |
| **Places searched** | `~/Library/{Application Support, Caches, Preferences, ByHost, Containers, Group Containers, Saved Application State, HTTPStorages, WebKit, Logs, LaunchAgents, …}`, `/Library/{Application Support, LaunchAgents, LaunchDaemons, PrivilegedHelperTools, Audio plug-ins, …}`, the per-user `/var/folders/…/C` and `/T` caches, home dot-folders | `%APPDATA%`, `%LOCALAPPDATA%`, `LocalLow`, `%ProgramData%`, Program Files (both), Start Menu, Desktop shortcuts, home dot-folders, and **registry** keys under `HKCU\Software` / `HKLM\SOFTWARE` (+WOW6432Node) |
| **Extras** | launch agents/daemons are also matched by what they *run*; installer receipts (`pkgutil`) are reported | the app's own uninstaller runs first (services and drivers come out cleanly), then sweepr sweeps the rest; registry keys are exported to a `.reg` backup before deletion |

Each match gets a confidence:

- **high**: exact bundle ID or app name. Selected by default.
- **medium**: prefix/partial matches, home dot-folders, AI-suggested names. Shown but not selected.
- **shared**: also matches another installed app, so it is never pre-selected.

Vendor folders (`Application Support/Google`, `AppData\Roaming\JetBrains`) are looked *inside*. The whole
folder is only removed when everything in it belongs to the app and no other installed app shares the vendor.

Uninstalls go to the **Trash / Recycle Bin** by default (`--permanent` to skip it). Root-owned
leftovers on macOS can be removed with `sudo` after a separate confirmation.

If you already dragged the app to the Trash, run `sweepr uninstall "App Name"` anyway: sweepr offers to
search for its leftovers by name. `sweepr leftovers` finds data from *any* app that is no longer installed.

## AI features (optional)

Set `ANTHROPIC_API_KEY` (or run `ant auth login`). Without credentials everything else works as normal.

| Command | What Claude does |
|---|---|
| `sweepr uninstall <app> --ai` | 1) suggests other names the app uses on disk (helper IDs, launch-agent labels, dot-folders), which sweepr then searches for; 2) reviews every candidate and marks it remove / keep / unsure with a reason |
| `sweepr leftovers --ai` | reviews orphaned data and pre-selects only what is clearly safe |
| `sweepr ai explain <path>` | says what a folder is, who owns it, and whether it is safe to delete (also `e N` inside `sweepr analyze`) |
| `sweepr ai advise` | measures the machine and returns a prioritized cleanup plan with sweepr commands |

Guardrails:

- **Only metadata leaves the machine:** paths (with your home folder shortened to `~`), names and sizes. File contents are never sent.
- **The AI never deletes.** Its verdicts only change which rows are pre-selected; you still review and confirm.
  A "remove" verdict pre-selects an item only at ≥ 75 % confidence and when no other installed app matches.
- **It can't reach outside the scan.** Verdicts refer to scanned items by ID, so the model cannot add a path,
  and every path still passes the safety checks below.
- Model: `claude-opus-5-5` by default; override with `SWEEPR_AI_MODEL`. Responses use structured JSON output,
  and server-side fallbacks are enabled for the default model.

## Safety

- **Protected locations:** the filesystem root, your home and its standard folders, `~/Library` and every top-level
  data folder inside it, `/System`, `/usr`, `/Applications` itself, `C:\Windows` (except its Temp and
  update-download folders), Program Files, `AppData` roots, keychains, iCloud Drive, Mail, Messages, `.ssh`,
  Windows credential stores. Neither these nor any folder containing them can be removed, whatever proposes it.
- **Whitelist:** `sweepr whitelist add ~/Library/Caches/SomethingImportant` (globs allowed).
- **Non-interactive runs** (pipes, scripts) never delete without `--yes`.
- **Operations log:** every removal is logged to `~/.config/sweepr/operations.log` (`%APPDATA%\sweepr\` on
  Windows). View it with `sweepr history`. Set `SWEEPR_NO_OPLOG=1` to disable logging.
- Symlinks are removed as links and never followed.

macOS hides some folders (the Trash, Safari, other apps' containers) unless your terminal has **Full Disk
Access** (System Settings → Privacy & Security). sweepr reports these as "no access" or a size of `?`
instead of failing.

## Layout

```
cmd/sweepr/          CLI: commands, prompts, AI wiring
internal/apps/       app discovery (Info.plist / Uninstall registry), leftover matcher, orphan finder
internal/clean/      junk targets per OS (targets_darwin.go, targets_windows.go, targets_other.go)
internal/safety/     the only code that deletes: validation, Trash/Recycle Bin, registry backup, oplog
internal/ai/         Claude client (official Go SDK) and the four AI tasks
internal/purge/      project build-artifact finder
internal/installer/  installer-file finder
internal/analyze/    disk usage explorer
internal/status/     disk/memory snapshot (syscalls, no cgo)
internal/ui/         colors, multi-select list, prompts
```

## Roadmap

- Microsoft Store (AppX/MSIX) apps and Homebrew casks in `uninstall`
- `optimize`: DNS flush, Spotlight/Launch Services rebuild, Windows component cleanup
- AI: learn from your keep/remove choices so future suggestions match how you work
- Self-update from GitHub releases
