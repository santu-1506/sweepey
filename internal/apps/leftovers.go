package apps

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/santu-1506/sweepey/internal/fsutil"
	"github.com/santu-1506/sweepey/internal/safety"
)

// Candidate is a file, folder or registry key that probably belongs to an app.
type Candidate struct {
	Path       string
	Kind       safety.Kind
	Size       int64
	Confidence Confidence
	Reason     string
	SharedWith string // another installed app that also matches
	Primary    bool   // the app bundle / install folder itself
	Selected   bool
	AINote     string
	AIVerdict  string // remove | keep | unsure
}

// root is one directory the leftover scan looks inside (one level deep, plus
// one more level inside vendor folders such as "JetBrains" or "Adobe").
type root struct {
	Dir          string
	MaxConf      Confidence // cap the confidence (home dotfiles are often user config)
	DotOnly      bool       // only consider names starting with "."
	Ext          string     // only consider this extension (e.g. ".lnk")
	ReadContents bool       // launchd plists: also match when the file points at the app
	CrashLogs    bool       // names look like App_2024-01-01-....ips
}

type named struct {
	name string
	m    *matcher
}

func othersFor(app App, all []App) []named {
	var out []named
	for _, o := range all {
		if (o.Path != "" && fsutil.SamePath(o.Path, app.Path)) ||
			(o.BundleID != "" && strings.EqualFold(o.BundleID, app.BundleID)) || o.Name == app.Name {
			continue
		}
		out = append(out, named{o.Name, newMatcher(o, nil)})
	}
	return out
}

// Leftovers finds everything that belongs to app. extraTerms are additional
// identifiers (e.g. suggested by the AI) that are matched with medium
// confidence so they are shown but never selected without review.
func Leftovers(app App, all []App, extraTerms []string) []Candidate {
	m := newMatcher(app, extraTerms)
	others := othersFor(app, all)
	var out []Candidate
	seen := map[string]bool{}
	add := func(c Candidate) {
		key := strings.ToLower(c.Path)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, c)
	}

	if app.Path != "" && fsutil.Exists(app.Path) && safety.Validate(app.Path) == nil {
		reason := "application bundle"
		if !strings.HasSuffix(strings.ToLower(app.Path), ".app") {
			reason = "install folder"
		}
		add(Candidate{Path: app.Path, Confidence: High, Reason: reason, Primary: true})
	}
	for _, r := range searchRoots() {
		scanRoot(r, m, others, add)
	}
	for _, c := range platformExtras(app, m, others) {
		add(c)
	}

	out = dropNested(out)
	markShared(out, others)
	measure(out)
	for i := range out {
		c := &out[i]
		c.Selected = c.Primary || (c.Confidence == High && c.SharedWith == "")
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Primary != b.Primary {
			return a.Primary
		}
		if a.Confidence != b.Confidence {
			return a.Confidence > b.Confidence
		}
		return a.Size > b.Size
	})
	return out
}

func scanRoot(r root, m *matcher, others []named, add func(Candidate)) {
	entries, err := os.ReadDir(r.Dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if name == ".DS_Store" || name == "desktop.ini" {
			continue
		}
		if r.DotOnly && !strings.HasPrefix(name, ".") {
			continue
		}
		if r.Ext != "" && !strings.EqualFold(filepath.Ext(name), r.Ext) {
			continue
		}
		p := filepath.Join(r.Dir, name)
		n := normalize(name)
		if r.CrashLogs {
			if i := strings.Index(n, "_"); i > 0 {
				n = n[:i]
			}
		}
		conf, reason := m.match(n)
		if conf == None && r.ReadContents && !e.IsDir() && m.referencedBy(p) {
			conf, reason = High, "launch item starts this app"
		}
		if conf > None {
			if r.MaxConf > None && conf > r.MaxConf {
				conf = r.MaxConf
			}
			add(Candidate{Path: p, Confidence: conf, Reason: reason})
			continue
		}
		if e.IsDir() && m.isVendor(n) {
			scanVendor(p, n, r, m, others, add)
		}
	}
}

// scanVendor looks inside a vendor folder. If every entry belongs to the app
// and no other installed app shares the vendor, the whole folder goes;
// otherwise only the matching entries do.
func scanVendor(dir, vendor string, r root, m *matcher, others []named, add func(Candidate)) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var matched []Candidate
	total, allHigh := 0, true
	for _, e := range entries {
		if e.Name() == ".DS_Store" || e.Name() == "desktop.ini" {
			continue
		}
		total++
		conf, reason := m.match(normalize(e.Name()))
		if conf == None {
			continue
		}
		if conf != High {
			allHigh = false
		}
		if r.MaxConf > None && conf > r.MaxConf {
			conf = r.MaxConf
		}
		matched = append(matched, Candidate{Path: filepath.Join(dir, e.Name()), Confidence: conf, Reason: reason + " (inside vendor folder)"})
	}
	shared := false
	for _, o := range others {
		if o.m.isVendor(vendor) {
			shared = true
			break
		}
	}
	if total > 0 && len(matched) == total && allHigh && !shared && r.MaxConf == None {
		add(Candidate{Path: dir, Confidence: High, Reason: "vendor folder holding only this app's data"})
		return
	}
	for _, c := range matched {
		add(c)
	}
}

// referencedBy reports whether a small file (a launchd plist) mentions the
// app's bundle path or ID.
func (m *matcher) referencedBy(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	data, _ := io.ReadAll(io.LimitReader(f, 256*1024))
	data = bytes.ToLower(data)
	if m.appPath != "" && bytes.Contains(data, []byte(strings.ToLower(m.appPath))) {
		return true
	}
	return m.bundle != "" && bytes.Contains(data, []byte(m.bundle))
}

// dropNested removes candidates that live inside another candidate.
func dropNested(in []Candidate) []Candidate {
	sort.SliceStable(in, func(i, j int) bool { return len(in[i].Path) < len(in[j].Path) })
	var out []Candidate
	for _, c := range in {
		inside := false
		for _, k := range out {
			if k.Kind == c.Kind && fsutil.HasPathPrefix(c.Path, k.Path) {
				inside = true
				break
			}
		}
		if !inside {
			out = append(out, c)
		}
	}
	return out
}

func markShared(cands []Candidate, others []named) {
	for i := range cands {
		c := &cands[i]
		if c.Primary {
			continue
		}
		n := normalize(baseName(c.Path))
		for _, o := range others {
			if conf, _ := o.m.match(n); conf == High {
				c.SharedWith = o.name
				break
			}
		}
	}
}

func baseName(p string) string {
	p = strings.TrimRight(p, `/\`)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func measure(cands []Candidate) {
	var paths []string
	var idx []int
	for i, c := range cands {
		if c.Kind == safety.File {
			paths = append(paths, c.Path)
			idx = append(idx, i)
		}
	}
	for j, s := range fsutil.Sizes(paths) {
		cands[idx[j]].Size = s
	}
}

// Orphans finds data left by apps that are no longer installed. Nothing is
// pre-selected: these are suggestions for review (by the user or the AI).
func Orphans(all []App) []Candidate {
	var ms []named
	for _, a := range all {
		ms = append(ms, named{a.Name, newMatcher(a, nil)})
	}
	var out []Candidate
	for _, r := range orphanRoots() {
		entries, err := os.ReadDir(r.Dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			n := normalize(e.Name())
			conf, reason := orphanCheck(n, e.IsDir(), ms)
			if conf == None {
				continue
			}
			p := filepath.Join(r.Dir, e.Name())
			if safety.Validate(p) != nil {
				continue
			}
			out = append(out, Candidate{Path: p, Confidence: conf, Reason: reason})
		}
	}
	measure(out)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Size > out[j].Size })
	return out
}

func ownedByInstalled(n string, ms []named) bool {
	for _, x := range ms {
		if conf, _ := x.m.match(n); conf >= Medium {
			return true
		}
		if x.m.isVendor(n) {
			return true
		}
	}
	return false
}
