package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"

	"github.com/anthropics/anthropic-sdk-go"
)

func osName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	}
	return "Linux"
}

// AppInfo is what the AI learns about the app being removed.
type AppInfo struct {
	Name       string   `json:"name"`
	BundleID   string   `json:"bundle_id,omitempty"`
	Executable string   `json:"executable,omitempty"`
	Vendor     string   `json:"vendor,omitempty"`
	Version    string   `json:"version,omitempty"`
	Installed  bool     `json:"still_installed"`
	SameVendor []string `json:"other_installed_apps_from_same_vendor,omitempty"`
}

// Term is an extra identifier the app is known to use on disk.
type Term struct {
	Term string `json:"term"`
	Why  string `json:"why"`
}

// SuggestTerms asks which other names the app uses for its files (helper
// bundle IDs, launch agent labels, dot-folders…) so the scanner can look
// for them too. Anything found this way is only medium confidence.
func (c *Client) SuggestTerms(ctx context.Context, app AppInfo) ([]Term, error) {
	system := `You help sweepr, a disk cleanup tool, find every file an application leaves behind on ` + osName() + `.
Given an app, list additional identifiers it is known to use for files, folders` + map[bool]string{true: ", registry keys", false: ""}[runtime.GOOS == "windows"] + ` and background items, beyond its display name and bundle ID:
helper and updater bundle IDs, launch agent labels, alternate folder names, dot-folders in the home directory, cache folder names, vendor sub-folder names.
Only list names you are confident are specific to this product. Never list generic words, the vendor name alone, or names shared with the vendor's other products (other installed apps from the same vendor are listed).
Return at most 15 terms. If you do not know the app, return an empty list.`
	payload, _ := json.MarshalIndent(app, "", "  ")
	schema := obj(map[string]any{
		"terms": arr(obj(map[string]any{
			"term": str("an exact file/folder name or reverse-DNS identifier, without paths"),
			"why":  str("short reason"),
		}, "term", "why")),
	}, "terms")
	var out struct {
		Terms []Term `json:"terms"`
	}
	err := c.structured(ctx, system, "Application:\n"+string(payload), schema, anthropic.BetaOutputConfigEffortLow, &out)
	if len(out.Terms) > 15 {
		out.Terms = out.Terms[:15]
	}
	return out.Terms, err
}

// Item is one candidate shown to the AI. ID is used instead of the path in
// the reply so the model cannot introduce a path that was not scanned.
type Item struct {
	ID         int    `json:"id"`
	Path       string `json:"path"`
	Kind       string `json:"kind"`
	SizeBytes  int64  `json:"size_bytes"`
	Heuristic  string `json:"heuristic_confidence"`
	Reason     string `json:"heuristic_reason"`
	SharedWith string `json:"also_matches_installed_app,omitempty"`
}

type Verdict struct {
	ID         int     `json:"id"`
	Decision   string  `json:"decision"` // remove | keep | unsure
	Confidence float64 `json:"confidence"`
	Reason     string  `json:"reason"`
}

var verdictSchema = obj(map[string]any{
	"verdicts": arr(obj(map[string]any{
		"id":         integer("the item id"),
		"decision":   enum("remove only if it belongs exclusively to the app and holds nothing the user would miss", "remove", "keep", "unsure"),
		"confidence": num("0.0 to 1.0"),
		"reason":     str("one short sentence"),
	}, "id", "decision", "confidence", "reason")),
}, "verdicts")

const reviewRules = `Rules:
- "keep" anything that may hold user-created data (documents, projects, notes, saves, exported files, license keys), anything shared with other installed software, vendor-wide frameworks or updaters still used by other products, and anything that belongs to the operating system.
- "remove" caches, logs, preferences, saved state, containers, crash reports, launch agents/daemons, helper tools and install folders that belong only to the app.
- "unsure" when the name alone does not tell you.
- You only see paths and sizes, never contents. Be conservative: a wrong "remove" loses data, a wrong "keep" only wastes space.
Return one verdict for every id.`

// ReviewLeftovers judges the candidates found for an app being uninstalled.
func (c *Client) ReviewLeftovers(ctx context.Context, app AppInfo, items []Item) ([]Verdict, error) {
	system := `You are the safety reviewer inside sweepr, a disk cleanup tool on ` + osName() + `.
The user is uninstalling the application below, and sweepr's name-matching scanner found possible leftover files.
For each item decide whether it belongs to this application and is safe to delete once the app is gone.
` + reviewRules
	payload, _ := json.MarshalIndent(map[string]any{"application": app, "items": items}, "", "  ")
	var out struct {
		Verdicts []Verdict `json:"verdicts"`
	}
	err := c.structured(ctx, system, string(payload), verdictSchema, anthropic.BetaOutputConfigEffortMedium, &out)
	return out.Verdicts, err
}

// ReviewOrphans judges data whose app no longer seems to be installed.
func (c *Client) ReviewOrphans(ctx context.Context, installed []string, items []Item) ([]Verdict, error) {
	system := `You are the safety reviewer inside sweepr, a disk cleanup tool on ` + osName() + `.
sweepr found folders and files in application-data locations that no installed application claims. Many are true leftovers from deleted apps; some belong to command-line tools, background services, system components, or apps installed in ways sweepr cannot see.
Using the installed-application list for context, decide for each item whether it is an orphan that is safe to delete.
` + reviewRules
	payload, _ := json.MarshalIndent(map[string]any{"installed_applications": installed, "items": items}, "", "  ")
	var out struct {
		Verdicts []Verdict `json:"verdicts"`
	}
	err := c.structured(ctx, system, string(payload), verdictSchema, anthropic.BetaOutputConfigEffortMedium, &out)
	return out.Verdicts, err
}

// Explanation describes an unknown file or folder.
type Explanation struct {
	What        string `json:"what"`
	Owner       string `json:"owner"`
	SafeToClean string `json:"safe_to_delete"`
	Explanation string `json:"explanation"`
	HowToRemove string `json:"how_to_remove"`
}

// Explain tells the user what a path is and whether it is safe to delete.
func (c *Client) Explain(ctx context.Context, path string, sizeBytes int64, isDir bool, children []string) (Explanation, error) {
	system := `You help users of sweepr, a disk cleanup tool on ` + osName() + `, understand what a file or folder is.
You see its path, size and (for folders) the names of its largest entries, never file contents.
Say what it most likely is, which software owns it, whether deleting it is safe, and the cleanest way to remove it (prefer the owning app's own setting or uninstaller, or a sweepr command: clean, uninstall, purge, installer). Say plainly when you are guessing.`
	payload, _ := json.MarshalIndent(map[string]any{"path": path, "size_bytes": sizeBytes, "is_folder": isDir,
		"largest_entries": children}, "", "  ")
	schema := obj(map[string]any{
		"what":           str("one line: what this is"),
		"owner":          str("software or component that owns it, or 'unknown'"),
		"safe_to_delete": enum("", "yes", "no", "depends"),
		"explanation":    str("2-4 sentences"),
		"how_to_remove":  str("recommended way to reclaim the space, or why not to"),
	}, "what", "owner", "safe_to_delete", "explanation", "how_to_remove")
	var out Explanation
	err := c.structured(ctx, system, string(payload), schema, anthropic.BetaOutputConfigEffortMedium, &out)
	return out, err
}

// Recommendation is one step of the cleanup plan.
type Recommendation struct {
	Title    string `json:"title"`
	Command  string `json:"command"`
	Savings  string `json:"estimated_savings"`
	Risk     string `json:"risk"`
	Detail   string `json:"detail"`
	Priority int    `json:"priority"`
}

type Advice struct {
	Summary         string           `json:"summary"`
	Recommendations []Recommendation `json:"recommendations"`
}

// Advise turns a machine snapshot into a prioritized cleanup plan.
func (c *Client) Advise(ctx context.Context, snapshot any) (Advice, error) {
	system := `You are the cleanup advisor inside sweepr, a disk cleanup tool on ` + osName() + `.
You receive a snapshot of the machine: disk and memory status, what "sweepr clean" would free per category, the largest folders in the home directory, installer files, project build artifacts, and apps.
Write a short, prioritized plan (at most 8 steps) to reclaim the most space with the least risk.
Each step's command must be a sweepr command (sweepr clean, sweepr uninstall "<App>", sweepr leftovers, sweepr purge, sweepr installer, sweepr analyze <path>) or a short manual instruction. Never suggest deleting personal files outright; for large personal folders suggest reviewing them with "sweepr analyze".
Be concrete and use the numbers from the snapshot.`
	payload, _ := json.MarshalIndent(snapshot, "", "  ")
	schema := obj(map[string]any{
		"summary": str("2-3 sentences on the machine's state"),
		"recommendations": arr(obj(map[string]any{
			"title":             str("short imperative title"),
			"command":           str("command to run, or empty"),
			"estimated_savings": str("e.g. '12 GB' or 'unknown'"),
			"risk":              enum("", "low", "medium", "high"),
			"detail":            str("one or two sentences"),
			"priority":          integer("1 = do first"),
		}, "title", "command", "estimated_savings", "risk", "detail", "priority")),
	}, "summary", "recommendations")
	var out Advice
	err := c.structured(ctx, system, fmt.Sprintf("Snapshot:\n%s", payload), schema, anthropic.BetaOutputConfigEffortMedium, &out)
	return out, err
}
