package apps

import (
	"regexp"
	"strings"
	"unicode"
)

// Confidence is how sure the heuristic is that a file belongs to the app.
type Confidence int

const (
	None Confidence = iota
	Low
	Medium
	High
)

func (c Confidence) String() string {
	return [...]string{"none", "low", "medium", "high"}[c]
}

// genericNames are too common to identify an app on their own (VS Code's
// executable is literally called "Electron").
var genericNames = map[string]bool{
	"app": true, "apps": true, "electron": true, "helper": true, "launcher": true, "updater": true, "update": true,
	"setup": true, "install": true, "installer": true, "uninstall": true, "uninstaller": true, "main": true,
	"run": true, "start": true, "java": true, "python": true, "node": true, "cache": true, "caches": true,
	"data": true, "config": true, "settings": true, "common": true, "shared": true, "support": true,
	"logs": true, "log": true, "temp": true, "tmp": true, "program": true, "programs": true, "microsoft": true,
	"google": true, "apple": true, "windows": true, "system": true, "user": true, "users": true,
	"default": true, "local": true, "bin": true, "lib": true, "tools": true, "service": true, "services": true,
	"agent": true, "daemon": true, "client": true, "desktop": true, "crashpad": true, "packages": true,
	"plugins": true, "extensions": true, "profiles": true, "preferences": true, "mac": true, "macos": true,
	"osx": true, "win": true, "win64": true, "x64": true, "x86": true, "inc": true, "llc": true, "ltd": true,
	"corp": true, "corporation": true, "software": true, "studio": true, "pro": true, "free": true, "lite": true,
}

var (
	teamPrefix  = regexp.MustCompile(`^[A-Z0-9]{10}\.`)
	bundleLike  = regexp.MustCompile(`^[a-z0-9-]+(\.[a-z0-9_-]+){2,}$`)
	vendorNoise = regexp.MustCompile(`(?i)[,.]?\s+(inc\.?|llc|ltd\.?|limited|corporation|corp\.?|gmbh|s\.?a\.?|co\.?|pty|b\.?v\.?|ag)$`)
	stripExts   = []string{".plist", ".savedstate", ".binarycookies", ".lnk", ".url", ".app", ".log", ".lockfile", ".ips", ".crash", ".diag"}
)

// normalize turns a file name into a comparable identifier: team-ID prefixes
// (ABCDE12345.com.foo) and group. prefixes are dropped, extensions stripped.
func normalize(name string) string {
	n := teamPrefix.ReplaceAllString(name, "")
	n = strings.ToLower(n)
	for _, ext := range stripExts {
		n = strings.TrimSuffix(n, ext)
	}
	return strings.TrimPrefix(n, "group.")
}

// compact drops separators so "Visual Studio Code", "visual-studio-code"
// and "VisualStudioCode" compare equal.
func compact(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '_', '.', '(', ')', '+':
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}

// matcher holds every identifier we know for one app.
type matcher struct {
	bundle  string   // lower-case bundle ID
	ids     []string // extra bundle-style IDs (from AI suggestions)
	exact   []string // compact names that identify the app outright
	weak    []string // compact names that only count as a medium match
	extra   []string // compact AI-suggested names (medium until reviewed)
	vendors []string // compact vendor folder names to look inside
	appPath string
}

func newMatcher(a App, extraTerms []string) *matcher {
	m := &matcher{bundle: strings.ToLower(a.BundleID), appPath: a.Path}
	addName := func(s string) {
		c := compact(s)
		if len(c) < 3 || genericNames[c] {
			return
		}
		if len(c) < 4 {
			m.weak = appendUnique(m.weak, c)
			return
		}
		m.exact = appendUnique(m.exact, c)
	}
	addName(a.Name)
	addName(a.BundleName)
	addName(a.Executable)
	addName(strings.TrimSuffix(a.Name, ".app"))
	// Names with a version or edition tail ("Docker Desktop", "Zoom Workplace")
	// often store data under just the first word.
	if first, _, ok := strings.Cut(a.Name, " "); ok {
		if c := compact(first); len(c) >= 4 && !genericNames[c] {
			m.weak = appendUnique(m.weak, c)
		}
	}
	if m.bundle != "" {
		parts := strings.Split(m.bundle, ".")
		if last := compact(parts[len(parts)-1]); len(last) >= 5 && !genericNames[last] {
			m.exact = appendUnique(m.exact, last)
		}
		if len(parts) >= 3 {
			// Big vendors ("microsoft", "google") are fine here: vendor folders
			// are only looked inside, never removed while shared.
			if v := compact(parts[1]); len(v) >= 3 {
				m.vendors = appendUnique(m.vendors, v)
			}
		}
	}
	if a.Vendor != "" {
		v := strings.TrimSpace(vendorNoise.ReplaceAllString(a.Vendor, ""))
		for _, c := range []string{compact(v), compact(a.Vendor)} {
			if len(c) >= 3 {
				m.vendors = appendUnique(m.vendors, c)
			}
		}
	}
	for _, t := range extraTerms {
		t = strings.TrimSpace(strings.ToLower(t))
		if bundleLike.MatchString(t) {
			m.ids = appendUnique(m.ids, t)
			continue
		}
		if c := compact(strings.TrimPrefix(t, ".")); len(c) >= 3 && !genericNames[c] {
			m.extra = appendUnique(m.extra, c)
		}
	}
	return m
}

func appendUnique(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

// match scores a normalized entry name.
func (m *matcher) match(n string) (Confidence, string) {
	if m.bundle != "" {
		if n == m.bundle || strings.HasPrefix(n, m.bundle+".") || strings.HasSuffix(n, "."+m.bundle) {
			return High, "matches bundle ID " + m.bundle
		}
	}
	c := compact(n)
	for _, e := range m.exact {
		if c == e {
			return High, "matches app name"
		}
	}
	for _, id := range m.ids {
		if n == id || strings.HasPrefix(n, id+".") {
			return Medium, "matches AI-suggested ID " + id
		}
	}
	for _, e := range m.extra {
		if c == e {
			return Medium, "matches AI-suggested name"
		}
	}
	for _, w := range m.weak {
		if c == w {
			return Medium, "matches short app name"
		}
	}
	for _, e := range m.exact {
		if len(e) >= 5 && strings.HasPrefix(c, e) {
			return Medium, "starts with app name"
		}
		if len(e) >= 7 && strings.Contains(c, e) {
			return Medium, "contains app name"
		}
	}
	return None, ""
}

func (m *matcher) isVendor(n string) bool {
	c := compact(n)
	for _, v := range m.vendors {
		if c == v {
			return true
		}
	}
	return false
}

// looksLikeBundleID reports names such as com.vendor.product.
func looksLikeBundleID(n string) bool { return bundleLike.MatchString(n) }
