package safety

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/santu-1506/sweepey/internal/fsutil"
)

func TestValidateRefuses(t *testing.T) {
	h := fsutil.Home()
	bad := []string{"", "relative/path", "/", h, filepath.Dir(h), filepath.Join(h, "Documents"), os.TempDir()}
	if runtime.GOOS == "darwin" {
		bad = append(bad, "/System/Library", "/usr/bin/ls", "/Applications", "/Library", "/Library/Application Support",
			filepath.Join(h, "Library"), filepath.Join(h, "Library/Caches"), filepath.Join(h, "Library/Keychains/login.keychain-db"),
			filepath.Join(h, "Library/Mobile Documents/com~apple~CloudDocs"), filepath.Join(h, ".ssh/id_ed25519"))
	}
	for _, p := range bad {
		if err := Validate(p); err == nil {
			t.Errorf("Validate(%q) should refuse", p)
		}
	}
}

func TestValidateAllows(t *testing.T) {
	h := fsutil.Home()
	good := []string{filepath.Join(os.TempDir(), "sweepr-test")}
	if runtime.GOOS == "darwin" {
		good = append(good, "/Applications/Slack.app", filepath.Join(h, "Library/Caches/com.tinyspeck.slackmacgap"),
			filepath.Join(h, "Library/Application Support/Slack"), "/Library/LaunchDaemons/com.docker.vmnetd.plist",
			filepath.Join(h, "Projects/app/node_modules"))
	}
	for _, p := range good {
		if err := Validate(p); err != nil {
			t.Errorf("Validate(%q) = %v, want ok", p, err)
		}
	}
}

func TestRemoveDryRunTouchesNothing(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "junk")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := Remover{DryRun: true}.Remove([]Target{{Path: f, Size: 1}})
	if len(res) != 1 || res[0].Err != nil {
		t.Fatalf("unexpected result %+v", res)
	}
	if !fsutil.Exists(f) {
		t.Fatal("dry run deleted the file")
	}
}

func TestRemoveDeletes(t *testing.T) {
	t.Setenv("SWEEPR_NO_OPLOG", "1")
	dir := t.TempDir()
	f := filepath.Join(dir, "junk")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	res := Remover{}.Remove([]Target{{Path: f, Size: 5}})
	freed, ok, failed, _ := Summary(res)
	if ok != 1 || failed != 0 || freed != 5 || fsutil.Exists(f) {
		t.Fatalf("delete failed: %+v", res)
	}
}
