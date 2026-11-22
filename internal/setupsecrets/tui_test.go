package setupsecrets

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// syncTUI runs jobs and widget changes right away, in place of a running application
func syncTUI(config *Config) *tui {
	t := newTUI(tview.NewApplication(), config)
	// One at a time, as on the event loop
	var mu sync.Mutex
	t.queue = func(f func()) { mu.Lock(); defer mu.Unlock(); f() }
	t.do = func(job func()) { job() }
	// tview calls it from a goroutine of its own, which queue would run there and then
	t.logs.SetChangedFunc(nil)
	return t
}

func TestTUISave(t *testing.T) {
	dir := t.TempDir()
	config := testConfig(t, `{
		"sources": {
			"STORED": {"cmd": "cat `+dir+`/stored 2>/dev/null || echo old", "storedIn": "`+dir+`/stored"},
			"EXTRACTED": {"cmd": "echo up"},
			"MANUAL": {}
		},
		"destinations": [
			{"logPrefix": "stored", "requires": ["STORED"], "cmd": "printf %s \"$STORED\" >`+dir+`/stored"},
			{"logPrefix": "both", "requires": ["STORED", "MANUAL"], "cmd": "printf '%s %s' \"$STORED\" \"$MANUAL\" >`+dir+`/both"},
			{"logPrefix": "extracted", "requires": ["EXTRACTED"], "cmd": "true"}
		]}`)
	ui := syncTUI(config)
	ui.fetchAll()
	if ui.srcState["STORED"] != stateFetched || ui.srcState["MANUAL"] != stateWriteOnly {
		t.Fatalf("states after the fetch: %v", ui.srcState)
	}
	if config.Sources["EXTRACTED"].editable() || !config.Sources["STORED"].editable() || !config.Sources["MANUAL"].editable() {
		t.Error("editable: only extracted sources are read-only")
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

func TestModalTakesTheMouse(t *testing.T) {
	config := testConfig(t, `{
		"sources": {"A": {}, "B": {}, "C": {}, "D": {}},
		"destinations": [{"logPrefix": "D", "requires": ["A", "B", "C", "D"], "cmd": "true"}]}`)
	ui := syncTUI(config)
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(160, 30)
	draw := func() {
		ui.pages.SetRect(0, 0, 160, 30)
		ui.pages.Draw(sim)
	}
	draw()
	// The NAME cell of the fourth secret, at the left edge, outside any modal
	x, y, _, _ := ui.secrets.GetRect()
	click := func() {
		event := tcell.NewEventMouse(x+6, y+5, tcell.Button1, 0)
		for _, action := range []tview.MouseAction{tview.MouseLeftDown, tview.MouseLeftUp, tview.MouseLeftClick} {
			ui.pages.MouseHandler()(action, event, func(tview.Primitive) {})
		}
	}
	ui.secrets.Select(1, 0)
	click()
	if row, _ := ui.secrets.GetSelection(); row != 4 {
		t.Fatalf("without a modal the click selects row 4, got %d", row)
	}

	ui.secrets.Select(1, 0)
	ui.render()
	ui.edit()
	draw()
	click()
	if row, _ := ui.secrets.GetSelection(); row != 1 {
		t.Errorf("the click went through the modal to row %d", row)
	}
	if front, _ := ui.pages.GetFrontPage(); front != "edit" {
		t.Errorf("front page %q", front)
	}
	// What's behind the modal is drawn muted
	_, _, style, _ := sim.GetContent(x+6, y+2)
	if fg, _, _ := style.Decompose(); fg != colorMuted {
		t.Errorf("the background isn't dimmed: %v", fg)
	}
}

func TestModalDimsEveryRow(t *testing.T) {
	config := testConfig(t, `{"sources": {"A": {}}, "destinations": [
		{"logPrefix": "one", "requires": ["A"], "cmd": "true"}, {"logPrefix": "two", "requires": ["A"], "cmd": "true"},
		{"logPrefix": "three", "requires": ["A"], "cmd": "true"}]}`)
	ui := syncTUI(config)
	sim := tcell.NewSimulationScreen("UTF-8")
	sim.Init()
	sim.SetSize(160, 30)
	ui.render()
	ui.edit()
	ui.pages.SetRect(0, 0, 160, 30)
	ui.pages.Draw(sim)
	x, y, _, _ := ui.dests.GetRect()
	for row := 1; row <= 3; row++ {
		for dx := 4; dx < 12; dx++ {
			mainc, _, style, _ := sim.GetContent(x+dx, y+1+row)
			if fg, _, _ := style.Decompose(); mainc != ' ' && fg != colorMuted {
				t.Errorf("row %d, column %d (%q) isn't dimmed: %v", row, dx, mainc, fg)
			}
		}
	}
}

func TestClickActivatesPane(t *testing.T) {
	config := testConfig(t, `{"sources": {"A": {}, "B": {}}, "destinations": [
		{"logPrefix": "one", "requires": ["A"], "cmd": "true"}, {"logPrefix": "two", "requires": ["B"], "cmd": "true"},
		{"logPrefix": "three", "requires": ["A"], "cmd": "true"}]}`)
	ui := syncTUI(config)
	sim := tcell.NewSimulationScreen("UTF-8")
	sim.Init()
	sim.SetSize(160, 30)
	ui.pages.SetRect(0, 0, 160, 30)
	ui.pages.Draw(sim)
	click := func(table *tview.Table, row int) {
		x, y, _, _ := table.GetRect()
		event := tcell.NewEventMouse(x+6, y+1+row, tcell.Button1, 0)
		for _, action := range []tview.MouseAction{tview.MouseLeftDown, tview.MouseLeftUp, tview.MouseLeftClick} {
			ui.pages.MouseHandler()(action, event, func(tview.Primitive) {})
		}
	}
	click(ui.dests, 3)
	if !ui.focusDests {
		t.Fatal("a click on the destinations didn't make them active")
	}
	if row, _ := ui.dests.GetSelection(); row != 3 {
		t.Errorf("the click selected destination row %d", row)
	}
	click(ui.secrets, 2)
	if ui.focusDests {
		t.Error("a click on the secrets didn't make them active")
	}
	if row, _ := ui.secrets.GetSelection(); row != 2 {
		t.Errorf("the click selected secret row %d", row)
	}
}

func TestDoubleClickDescribes(t *testing.T) {
	config := testConfig(t, `{"sources": {"A": {}, "B": {}}, "destinations": [{"logPrefix": "one", "requires": ["A", "B"], "cmd": "true"}]}`)
	ui := syncTUI(config)
	sim := tcell.NewSimulationScreen("UTF-8")
	sim.Init()
	sim.SetSize(160, 30)
	ui.pages.SetRect(0, 0, 160, 30)
	ui.pages.Draw(sim)
	x, y, _, _ := ui.secrets.GetRect()
	event := tcell.NewEventMouse(x+6, y+1+2, tcell.Button1, 0)
	for _, action := range []tview.MouseAction{tview.MouseLeftDown, tview.MouseLeftUp, tview.MouseLeftClick, tview.MouseLeftDoubleClick} {
		ui.pages.MouseHandler()(action, event, func(tview.Primitive) {})
	}
	if front, _ := ui.pages.GetFrontPage(); front != "describe" {
		t.Fatalf("front page %q", front)
	}
	if ui.selectedSecret() != "B" {
		t.Errorf("described %q", ui.selectedSecret())
	}
}
