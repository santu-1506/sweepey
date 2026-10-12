package safety

import (
	"errors"
	"io/fs"
	"os"
	"strings"
)

type Kind int

const (
	File Kind = iota
	Registry
)

// Target is one thing to remove.
type Target struct {
	Path string
	Kind Kind
	Size int64
}

type Result struct {
	Target
	Err        error
	NeedsAdmin bool
}

// Remover deletes targets. Trash moves files to the Trash / Recycle Bin so
// they can be restored; otherwise they are deleted permanently.
type Remover struct {
	DryRun bool
	Trash  bool
	Op     string // recorded in the operations log, e.g. "clean"
}

func (r Remover) Remove(targets []Target) []Result {
	results := make([]Result, 0, len(targets))
	var files []Target
	for _, t := range targets {
		if t.Kind == Registry {
			if r.DryRun {
				results = append(results, Result{Target: t})
				continue
			}
			err := removeRegistryKey(t.Path)
			results = append(results, Result{Target: t, Err: err, NeedsAdmin: isAccessDenied(err)})
			continue
		}
		if err := Validate(t.Path); err != nil {
			results = append(results, Result{Target: t, Err: err})
			continue
		}
		files = append(files, t)
	}

	if r.DryRun {
		for _, t := range files {
			results = append(results, Result{Target: t})
		}
		return results
	}

	if r.Trash && len(files) > 0 {
		paths := make([]string, len(files))
		for i, t := range files {
			paths[i] = t.Path
		}
		errs := moveToTrash(paths)
		for _, t := range files {
			err := errs[t.Path]
			results = append(results, Result{Target: t, Err: err, NeedsAdmin: isAccessDenied(err)})
		}
	} else {
		for _, t := range files {
			err := os.RemoveAll(t.Path)
			results = append(results, Result{Target: t, Err: err, NeedsAdmin: isAccessDenied(err)})
		}
	}
	logResults(r, results)
	return results
}

func isAccessDenied(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, fs.ErrPermission) {
		return true
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "access is denied") || strings.Contains(s, "operation not permitted") ||
		strings.Contains(s, "permission denied")
}

// Summary totals what a run freed and how many items failed.
func Summary(results []Result) (freed int64, ok, failed, admin int) {
	for _, r := range results {
		switch {
		case r.Err == nil:
			ok++
			if r.Size > 0 {
				freed += r.Size
			}
		case r.NeedsAdmin:
			admin++
		default:
			failed++
		}
	}
	return
}
