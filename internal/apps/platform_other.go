//go:build !darwin && !windows

package apps

import "errors"

func discover() ([]App, error) {
	return nil, errors.New("app uninstall is supported on macOS and Windows; use your package manager on Linux")
}

func searchRoots() []root                                    { return nil }
func platformExtras(App, *matcher, []named) []Candidate      { return nil }
func orphanRoots() []root                                    { return nil }
func orphanCheck(string, bool, []named) (Confidence, string) { return None, "" }
func IsRunning(App) bool                                     { return false }
func Quit(App) error                                         { return nil }
func RunUninstaller(App, bool) error                         { return nil }
func Receipts(App) []string                                  { return nil }
