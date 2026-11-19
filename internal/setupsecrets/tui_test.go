package setupsecrets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rivo/tview"
)

// syncTUI runs jobs and widget changes right away, in place of a running application
func syncTUI(config *Config) *tui {
	t := newTUI(tview.NewApplication(), config)
	t.queue = func(f func()) { f() }
	t.do = func(job func()) { job() }
	return t
}

func TestTUISave(t *testing.T) {
	dir := t.TempDir()
	config := testConfig(t, `{
		"sources": {
			"STORED": {"cmd": "cat `+dir+`/stored 2>/dev/null || echo old", "storedIn": "`+dir+`/stored"},
			"UPSTREAM": {"cmd": "echo up"},
			"MANUAL": {}
		},
		"destinations": [
			{"logPrefix": "stored", "requires": ["STORED"], "cmd": "printf %s \"$STORED\" >`+dir+`/stored"},
			{"logPrefix": "both", "requires": ["STORED", "MANUAL"], "cmd": "printf '%s %s' \"$STORED\" \"$MANUAL\" >`+dir+`/both"},
			{"logPrefix": "upstream", "requires": ["UPSTREAM"], "cmd": "true"}
		]}`)
	ui := syncTUI(config)
	ui.fetchAll()
	if ui.srcState["STORED"] != stateFetched || ui.srcState["MANUAL"] != stateMissing {
		t.Fatalf("states after the fetch: %v", ui.srcState)
	}
	if config.Sources["UPSTREAM"].editable() || !config.Sources["STORED"].editable() || !config.Sources["MANUAL"].editable() {
		t.Error("editable: only upstream sources are read-only")
	}

	// Saving the manual value stores only the destination that needs it, with the other value it needs
	ui.save("MANUAL", "m")
	if got, _ := os.ReadFile(filepath.Join(dir, "both")); string(got) != "old m" {
		t.Errorf("both: %q", got)
	}
	if ui.destState[0] != stateNotRun || ui.destState[1] != stateStored || ui.destState[2] != stateNotRun {
		t.Errorf("destination states %v", ui.destState)
	}

	// An edit of a stored value sticks: the next fetch reads it back
	ui.save("STORED", "new")
	ui.fetchOne("STORED")
	if v := *config.Sources["STORED"].Value; v != "new" {
		t.Errorf("STORED after a fetch: %q", v)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "both")); string(got) != "new m" {
		t.Errorf("both: %q", got)
	}
	if !strings.Contains(ui.logs.GetText(false), "MANUAL: Saved, storing") {
		t.Errorf("log:\n%s", ui.logs.GetText(false))
	}
}
