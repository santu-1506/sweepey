package apps

import "testing"

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"com.tinyspeck.slackmacgap.plist": "com.tinyspeck.slackmacgap",
		"UBF8T346G9.com.microsoft.teams":  "com.microsoft.teams",
		"group.com.docker":                "com.docker",
		"com.google.Chrome.savedState":    "com.google.chrome",
		"Visual Studio Code.lnk":          "visual studio code",
	}
	for in, want := range cases {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatcher(t *testing.T) {
	vscode := App{Name: "Visual Studio Code", BundleName: "Code", BundleID: "com.microsoft.VSCode", Executable: "Electron"}
	m := newMatcher(vscode, nil)
	cases := []struct {
		entry string
		want  Confidence
	}{
		{"com.microsoft.VSCode.plist", High},
		{"com.microsoft.VSCode.ShipIt", High},
		{"Code", High},
		{"Visual Studio Code", High},
		{"Electron", None}, // generic executable name must not match
		{"com.microsoft.Word", None},
		{"Microsoft", None},
	}
	for _, c := range cases {
		if got, _ := m.match(normalize(c.entry)); got != c.want {
			t.Errorf("match(%q) = %v, want %v", c.entry, got, c.want)
		}
	}
	if !m.isVendor("microsoft") {
		t.Error("microsoft should be a vendor folder for com.microsoft.VSCode")
	}
}

func TestMatcherExtraTermsAreMedium(t *testing.T) {
	m := newMatcher(App{Name: "Visual Studio Code", BundleID: "com.microsoft.VSCode"}, []string{".code-helper-data", "com.microsoft.VSCodeHelper"})
	if got, _ := m.match(normalize(".code-helper-data")); got != Medium {
		t.Errorf("AI term = %v, want medium", got)
	}
	if got, _ := m.match(normalize("com.microsoft.vscodehelper.plist")); got != Medium {
		t.Errorf("AI id = %v, want medium", got)
	}
}

func TestWindowsPublisher(t *testing.T) {
	m := newMatcher(App{Name: "Zoom Workplace", Vendor: "Zoom Video Communications, Inc.", Executable: "Zoom"}, nil)
	if !m.isVendor("zoom video communications") {
		t.Error("publisher without the Inc. suffix should be a vendor folder")
	}
	if got, _ := m.match(normalize("Zoom")); got == None {
		t.Error("executable name should match")
	}
}

func TestFind(t *testing.T) {
	list := []App{{Name: "Slack"}, {Name: "Slack Helper"}, {Name: "Google Chrome", BundleID: "com.google.Chrome"}}
	if got := Find(list, "slack"); len(got) != 1 || got[0].Name != "Slack" {
		t.Errorf("exact match should win, got %v", got)
	}
	if got := Find(list, "chrome"); len(got) != 1 {
		t.Errorf("partial match failed, got %v", got)
	}
	if got := Find(list, "com.google.chrome"); len(got) != 1 {
		t.Errorf("bundle ID match failed, got %v", got)
	}
}

func TestLooksLikeBundleID(t *testing.T) {
	if !looksLikeBundleID("com.spotify.client") || looksLikeBundleID("Spotify") || looksLikeBundleID("com.foo") {
		t.Error("bundle ID detection is off")
	}
}
