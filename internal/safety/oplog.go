package safety

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/santu-1506/sweepey/internal/config"
)

// Entry is one line of the operations log (JSON Lines).
type Entry struct {
	Time  time.Time `json:"time"`
	Op    string    `json:"op"`
	Path  string    `json:"path"`
	Size  int64     `json:"size"`
	Mode  string    `json:"mode"`
	OK    bool      `json:"ok"`
	Error string    `json:"error,omitempty"`
}

func LogPath() string { return filepath.Join(config.Dir(), "operations.log") }

func logResults(r Remover, results []Result) {
	if os.Getenv("SWEEPR_NO_OPLOG") == "1" || len(results) == 0 {
		return
	}
	if err := os.MkdirAll(config.Dir(), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(LogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	mode := "delete"
	if r.Trash {
		mode = "trash"
	}
	enc := json.NewEncoder(f)
	now := time.Now()
	for _, res := range results {
		e := Entry{Time: now, Op: r.Op, Path: res.Path, Size: res.Size, Mode: mode, OK: res.Err == nil}
		if res.Kind == Registry {
			e.Mode = "registry"
		}
		if res.Err != nil {
			e.Error = res.Err.Error()
		}
		_ = enc.Encode(e)
	}
}

// ReadLog returns every recorded operation, oldest first.
func ReadLog() ([]Entry, error) {
	f, err := os.Open(LogPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Entry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	for sc.Scan() {
		var e Entry
		if json.Unmarshal(sc.Bytes(), &e) == nil {
			out = append(out, e)
		}
	}
	return out, sc.Err()
}
