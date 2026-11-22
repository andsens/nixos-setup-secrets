package setupsecrets

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

var (
	colorBg     = tcell.NewHexColor(0x0f1419)
	colorFg     = tcell.NewHexColor(0xc5cdd9)
	colorDim    = tcell.NewHexColor(0x8a96a3)
	colorMuted  = tcell.NewHexColor(0x3e4752)
	colorAccent = tcell.NewHexColor(0x59c2ff)
	colorKey    = tcell.NewHexColor(0xffb454)
	colorGreen  = tcell.NewHexColor(0x91d076)
	colorYellow = tcell.NewHexColor(0xe6c446)
	colorRed    = tcell.NewHexColor(0xf07178)
	colorCyan   = tcell.NewHexColor(0x95e6cb)
	colorSelBg  = tcell.NewHexColor(0x1f3a52)
	colorLinkBg = tcell.NewHexColor(0x172634)
	colorBorder = tcell.NewHexColor(0x2d3640)
)

// What a secret or destination shows in its STATE column
const (
	stateFetching = "fetching"
	stateFetched  = "fetched"
	stateFailed   = "failed"
	stateMissing  = "missing"
	stateSet      = "set"
	stateEdited   = "edited"
	stateNotRun   = "not run"
	stateStoring  = "storing"
	stateStored   = "stored"
	stateWaiting  = "waiting"
	// A secret without a command nobody typed in: its value only exists where its destination wrote it
	stateWriteOnly = "write-only"
	// A destination whose only gaps are such secrets
	stateUntouched = "untouched"
	// What the dry run marks the destinations with
	stateWouldRun  = "would run"
	stateUnchanged = "unchanged"
)

func stateColor(state string) tcell.Color {
	switch state {
	case stateFetched, stateStored, stateEdited:
		return colorGreen
	case stateFailed:
		return colorRed
	case stateMissing, stateWaiting, stateWouldRun:
		return colorYellow
	case stateSet:
		return colorCyan
	}
	return colorDim
}

// kind tells where the value of a source lives, which decides whether it can be edited
func (src *Source) kind() string {
	switch {
	case src.Cmd == "":
		return "manual"
	case src.StoredIn != "":
		return "stored"
	}
	return "extracted"
}

func (src *Source) editable() bool { return src.kind() != "extracted" }

type tui struct {
	host   string
	app    *tview.Application
	config *Config
	names  []string
	// What the STATE columns show, by secret name and by destination index
	srcState  map[string]string
	destState []string
	// The tokens of the same commands, by secret name, and why the dry run would run a destination, by index
	tokens  map[string]string
	reasons map[int]string

	pages          *tview.Pages
	header         *tview.TextView
	secrets, dests *tview.Table
	status, crumbs *tview.TextView
	logs           *tview.TextView
	focusDests     bool

	// Guards the values of the sources, which jobs write while the event loop reads them
	mu sync.RWMutex
	// queue runs a change of the widgets on the event loop, do runs a job off it, one at a time
	queue func(func())
	do    func(func())
}

func newTUI(app *tview.Application, config *Config) *tui {
	tview.Styles.PrimitiveBackgroundColor = colorBg
	tview.Styles.ContrastBackgroundColor = colorSelBg
	tview.Styles.MoreContrastBackgroundColor = colorBorder
	tview.Styles.BorderColor = colorBorder
	tview.Styles.TitleColor = colorAccent
	tview.Styles.PrimaryTextColor = colorFg
	tview.Styles.SecondaryTextColor = colorKey
	tview.Borders.HorizontalFocus = tview.Borders.Horizontal
	tview.Borders.VerticalFocus = tview.Borders.Vertical
	tview.Borders.TopLeftFocus = tview.Borders.TopLeft
	tview.Borders.TopRightFocus = tview.Borders.TopRight
	tview.Borders.BottomLeftFocus = tview.Borders.BottomLeft
	tview.Borders.BottomRightFocus = tview.Borders.BottomRight

	host, _ := os.Hostname()
	t := &tui{
		host:      host,
		app:       app,
		config:    config,
		names:     slices.Sorted(maps.Keys(config.Sources)),
		srcState:  map[string]string{},
		destState: make([]string, len(config.Destinations)),
		tokens:    map[string]string{},
		reasons:   map[int]string{},
		header:    tview.NewTextView().SetDynamicColors(true),
		secrets:   tview.NewTable(),
		dests:     tview.NewTable(),
		status:    tview.NewTextView().SetDynamicColors(true),
		crumbs:    tview.NewTextView().SetDynamicColors(true),
		logs:      tview.NewTextView().SetDynamicColors(false),
	}
	t.queue = func(f func()) { app.QueueUpdateDraw(f) }
	jobs := make(chan func(), 16)
	go func() {
		for job := range jobs {
			job()
		}
	}()
	t.do = func(job func()) { jobs <- job }

	for _, name := range t.names {
		t.srcState[name] = stateWriteOnly
		if config.Sources[name].Cmd != "" {
			t.srcState[name] = stateFetching
		}
	}
	for i := range t.destState {
		t.destState[i] = stateNotRun
	}

	for _, table := range []*tview.Table{t.secrets, t.dests} {
		table.SetSelectable(true, false).SetFixed(1, 0)
		table.SetSelectedStyle(tcell.StyleDefault.Background(colorSelBg).Foreground(tcell.ColorWhite))
		table.SetBorder(true).SetBorderPadding(0, 0, 1, 1)
		table.SetSelectionChangedFunc(func(int, int) { t.render() })
		// A click into the inactive pane makes it the active one, then selects the row clicked
		dests := table == t.dests
		// and a double click on a row describes it
		table.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
			switch action {
			case tview.MouseLeftDown:
				if t.focusDests != dests {
					t.focusPane(dests)
				}
			case tview.MouseLeftDoubleClick:
				if row, _ := table.CellAt(event.Position()); row >= 1 && row < table.GetRowCount() {
					table.Select(row, 0)
					t.describe()
					return tview.MouseConsumed, nil
				}
			}
			return action, event
		})
	}
	t.logs.SetBorder(true).SetTitle(" [::b]log[::-] ").SetBorderPadding(0, 0, 1, 1)
	t.logs.SetChangedFunc(func() { t.queue(func() { t.logs.ScrollToEnd() }) })

	body := tview.NewFlex().AddItem(t.secrets, 86, 0, true).AddItem(t.dests, 0, 1, false)
	main := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(t.header, 4, 0, false).
		AddItem(body, 0, 1, true).
		AddItem(t.status, 1, 0, false).
		AddItem(t.crumbs, 1, 0, false)
	t.pages = tview.NewPages().AddPage("main", main, true, true)
	main.SetInputCapture(t.mainKeys)
	t.secrets.Select(1, 0)
	t.dests.Select(1, 0)
	t.render()
	return t
}

// SetupManual opens the TUI: every secret beside every destination, edits stored as they are saved
func SetupManual(config *Config) error {
	app := tview.NewApplication().EnableMouse(true)
	t := newTUI(app, config)
	app.SetRoot(t.pages, true).SetFocus(t.secrets)
	t.do(t.fetchAll)
	return app.Run()
}

func (t *tui) selectedSecret() string {
	row, _ := t.secrets.GetSelection()
	if row < 1 || row > len(t.names) {
		return ""
	}
	return t.names[row-1]
}

func (t *tui) selectedDest() int {
	row, _ := t.dests.GetSelection()
	if row < 1 || row > len(t.config.Destinations) {
		return -1
	}
	return row - 1
}

// destsOf returns the indices of the destinations that need the secret
func (t *tui) destsOf(name string) []int {
	var out []int
	for i, dest := range t.config.Destinations {
		if dest.Needs(name) {
			out = append(out, i)
		}
	}
	return out
}

func (t *tui) hasValue(name string) bool {
	src, ok := t.config.Sources[name]
	return ok && src.Value != nil && *src.Value != ""
}

func (t *tui) groupsOf(name string) string {
	var groups []string
	for _, i := range t.destsOf(name) {
		if g := t.config.Destinations[i].Group; !slices.Contains(groups, g) {
			groups = append(groups, g)
		}
	}
	slices.Sort(groups)
	return strings.Join(groups, ",")
}

func (t *tui) ready(dest *Destination) (have, need int) {
	for _, name := range dest.Requires {
		if t.hasValue(name) {
			have++
		}
	}
	return have, len(dest.Requires)
}

func cell(text string, color tcell.Color, linked, dimmed bool) *tview.TableCell {
	c := tview.NewTableCell(text + "  ").SetTextColor(color)
	if dimmed {
		c.SetTextColor(colorMuted)
	}
	if linked {
		c.SetBackgroundColor(colorLinkBg)
	}
	return c
}

func header(table *tview.Table, cols ...string) {
	for i, col := range cols {
		table.SetCell(0, i, tview.NewTableCell(col).SetAttributes(tcell.AttrBold).SetSelectable(false).
			SetExpansion(map[bool]int{true: 1}[i == len(cols)-1]))
	}
}

// render redraws everything from the state: the tables with the rows linked to the selection highlighted, the
// header, the status line and the crumbs
func (t *tui) render() {
	t.mu.RLock()
	defer t.mu.RUnlock()
	linkedSecrets, linkedDests := map[string]bool{}, map[int]bool{}
	if t.focusDests {
		if i := t.selectedDest(); i >= 0 {
			for _, name := range append(slices.Clone(t.config.Destinations[i].Requires), t.config.Destinations[i].Wants...) {
				linkedSecrets[name] = true
			}
		}
	} else if name := t.selectedSecret(); name != "" {
		for _, i := range t.destsOf(name) {
			linkedDests[i] = true
		}
	}

	header(t.secrets, "", "NAME", "KIND", "STATE", "SAME", "GROUPS")
	for r, name := range t.names {
		src, linked := t.config.Sources[name], linkedSecrets[name]
		dimmed := t.focusDests && !linked
		mark, nameColor := " ", colorFg
		if linked {
			mark = "●"
		}
		if !src.editable() {
			nameColor = colorDim
		}
		state := t.srcState[name]
		t.secrets.SetCell(r+1, 0, cell(mark, colorAccent, linked, dimmed))
		t.secrets.SetCell(r+1, 1, cell(name, nameColor, linked, dimmed))
		t.secrets.SetCell(r+1, 2, cell(src.kind(), colorDim, linked, dimmed))
		t.secrets.SetCell(r+1, 3, cell(state, stateColor(state), linked, dimmed))
		t.secrets.SetCell(r+1, 4, cell(t.tokens[name], colorDim, linked, dimmed).SetMaxWidth(12))
		t.secrets.SetCell(r+1, 5, cell(t.groupsOf(name), colorDim, linked, dimmed))
	}
	header(t.dests, "", "DESTINATION", "GROUP", "STATE", "READY")
	for r, dest := range t.config.Destinations {
		linked := linkedDests[r]
		dimmed := !t.focusDests && !linked
		mark := " "
		if linked {
			mark = "●"
		}
		have, need := t.ready(dest)
		readyColor := colorDim
		if len(t.fetchableGaps(dest)) > 0 {
			readyColor = colorYellow
		}
		state := t.destState[r]
		t.dests.SetCell(r+1, 0, cell(mark, colorAccent, linked, dimmed))
		t.dests.SetCell(r+1, 1, cell(dest.LogPrefix, colorFg, linked, dimmed))
		t.dests.SetCell(r+1, 2, cell(dest.Group, colorDim, linked, dimmed))
		t.dests.SetCell(r+1, 3, cell(state, stateColor(state), linked, dimmed))
		t.dests.SetCell(r+1, 4, cell(fmt.Sprintf("%d of %d", have, need), readyColor, linked, dimmed))
	}

	focusTitle := func(title string, focused bool, count int) string {
		if focused {
			return fmt.Sprintf(" [::b]%s[::-][#ffb454][%d][-] ", title, count)
		}
		return fmt.Sprintf(" %s[#ffb454][%d][-] ", title, count)
	}
	t.secrets.SetTitle(focusTitle("secrets", !t.focusDests, len(t.names)))
	t.dests.SetTitle(focusTitle("destinations", t.focusDests, len(t.config.Destinations)))
	t.secrets.SetBorderColor(map[bool]tcell.Color{true: colorAccent, false: colorBorder}[!t.focusDests])
	t.dests.SetBorderColor(map[bool]tcell.Color{true: colorAccent, false: colorBorder}[t.focusDests])
	t.secrets.SetSelectable(!t.focusDests, false)
	t.dests.SetSelectable(t.focusDests, false)

	t.renderHeader()
	t.renderStatus(linkedDests)
}

func count(states []string, state string) int {
	n := 0
	for _, s := range states {
		if s == state {
			n++
		}
	}
	return n
}

func (t *tui) renderHeader() {
	srcStates := slices.Collect(maps.Values(t.srcState))
	part := func(n int, color, label string) string {
		if n == 0 {
			return ""
		}
		return fmt.Sprintf("[%s]%d %s[-]  ", color, n, label)
	}
	// What the keys do in the focused pane: secrets are edited, destinations only run
	hints := [][2]string{{"tab", "Destinations"}, {"e", "Edit value"}, {"r", "Retry"}, {"enter", "Describe"}, {"n", "Dry run"},
		{"o", "Orphans"}, {"l", "Log"}, {"q", "Quit"}}
	if t.focusDests {
		hints = [][2]string{{"tab", "Secrets"}, {"r", "Run"}, {"enter", "Describe"}, {"n", "Dry run"}, {"o", "Orphans"},
			{"l", "Log"}, {"q", "Quit"}}
	}
	var cols [3]strings.Builder
	for i, hint := range hints {
		fmt.Fprintf(&cols[i/3], "[#59c2ff::b]<%s>[-::-] [#8a96a3]%s[-]\n", hint[0], hint[1])
	}
	lines := []string{
		fmt.Sprintf("[#ffb454]Host:[-]          %s", t.host),
		"[#ffb454]Secrets:[-]       " + part(count(srcStates, stateFetched)+count(srcStates, stateSet)+count(srcStates, stateEdited), "#91d076", "with value") +
			part(count(srcStates, stateFailed), "#f07178", "failed") + part(count(srcStates, stateMissing), "#e6c446", "missing") + part(count(srcStates, stateWriteOnly), "#8a96a3", "write-only") +
			part(count(srcStates, stateFetching), "#8a96a3", "fetching"),
		"[#ffb454]Destinations:[-]  " + part(count(t.destState, stateStored), "#91d076", "stored") +
			part(count(t.destState, stateFailed), "#f07178", "failed") + part(count(t.destState, stateWaiting), "#e6c446", "waiting") +
			part(count(t.destState, stateNotRun), "#8a96a3", "not run"),
	}
	var hintLines [3][]string
	for c := range cols {
		hintLines[c] = strings.Split(strings.TrimSpace(cols[c].String()), "\n")
	}
	var sb strings.Builder
	for i := range 3 {
		left := ""
		if i < len(lines) {
			left = lines[i]
		}
		fmt.Fprintf(&sb, "%s", padTagged(left, 52))
		for c := range 3 {
			if i < len(hintLines[c]) {
				fmt.Fprintf(&sb, "%s", padTagged(hintLines[c][i], 24))
			}
		}
		if i == 0 {
			sb.WriteString("[#59c2ff::b]setup-secrets[-::-]")
		}
		sb.WriteString("\n")
	}
	t.header.SetText(sb.String())
}

// padTagged pads text with style tags to width visible characters
func padTagged(text string, width int) string {
	if n := tview.TaggedStringWidth(text); n < width {
		return text + strings.Repeat(" ", width-n)
	}
	return text
}

func (t *tui) renderStatus(linkedDests map[int]bool) {
	var status, crumb string
	if t.focusDests {
		if i := t.selectedDest(); i >= 0 {
			dest := t.config.Destinations[i]
			crumb = dest.LogPrefix
			var missing, writeOnly []string
			for _, name := range dest.Requires {
				switch {
				case t.hasValue(name):
				case t.config.Sources[name] != nil && t.config.Sources[name].Cmd == "":
					writeOnly = append(writeOnly, name)
				default:
					missing = append(missing, fmt.Sprintf("[%s]%s[#8a96a3] (%s)", colorName(stateColor(t.srcState[name])), name, t.srcState[name]))
				}
			}
			switch {
			case len(missing) > 0:
				status = dest.LogPrefix + " waits for " + strings.Join(missing, ", ")
			case len(writeOnly) > 0:
				status = dest.LogPrefix + " keeps what it has: nobody typed in " + strings.Join(writeOnly, ", ") + "; edit it in the secrets pane to set one"
			case t.destState[i] == stateWouldRun:
				status = dest.LogPrefix + " would run: " + tview.Escape(t.reasons[i])
			case t.destState[i] == stateUnchanged:
				status = dest.LogPrefix + " is unchanged since it last stored"
			default:
				status = dest.LogPrefix + " has all it requires; r runs it"
			}
		}
	} else if name := t.selectedSecret(); name != "" {
		crumb = name
		src := t.config.Sources[name]
		dests := fmt.Sprintf("its %d destinations", len(linkedDests))
		if len(linkedDests) == 1 {
			dests = "its destination"
		}
		switch src.kind() {
		case "extracted":
			status = fmt.Sprintf("%s comes from a command setup-secrets doesn't write to, so it's read-only; r fetches it again and runs %s", name, dests)
		case "stored":
			status = fmt.Sprintf("Saving %s runs %s; the value is read back from %s", name, dests, tview.Escape(src.StoredIn))
		default:
			status = fmt.Sprintf("Saving %s runs %s", name, dests)
		}
	}
	t.status.SetText(" [#8a96a3]" + status + "[-]")
	pane := "secrets"
	if t.focusDests {
		pane = "destinations"
	}
	t.crumbs.SetText(fmt.Sprintf("[#0f1419:#8a96a3:b] <%s> [-:-:-] [#0f1419:#ffb454:b] <%s> [-:-:-]", pane, tview.Escape(crumb)))
}

func colorName(c tcell.Color) string {
	r, g, b := c.RGB()
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

func (t *tui) mainKeys(event *tcell.EventKey) *tcell.EventKey {
	switch {
	case event.Key() == tcell.KeyTab || event.Key() == tcell.KeyBacktab:
		t.focusPane(!t.focusDests)
		return nil
	case event.Key() == tcell.KeyEnter:
		t.describe()
		return nil
	case event.Rune() == 'e':
		t.edit()
		return nil
	case event.Rune() == 'r':
		t.retry()
		return nil
	case event.Rune() == 'n':
		t.do(t.dryRun)
		return nil
	case event.Rune() == 'o':
		t.do(t.loadOrphans)
		return nil
	case event.Rune() == 'l':
		t.showLog()
		return nil
	case event.Rune() == 'q':
		t.app.Stop()
		return nil
	}
	return event
}

// focusPane makes the destinations pane the active one, or the secrets pane
func (t *tui) focusPane(dests bool) {
	t.focusDests = dests
	if dests {
		t.app.SetFocus(t.dests)
	} else {
		t.app.SetFocus(t.secrets)
	}
	t.render()
}

// page shows p over the main view; esc closes it
func (t *tui) page(name string, p tview.Primitive, overlay bool) {
	t.pages.AddPage(name, p, true, true)
	if !overlay {
		t.pages.SendToFront(name)
	}
}

func (t *tui) closePage(name string) {
	t.pages.RemovePage(name)
	if t.focusDests {
		t.app.SetFocus(t.dests)
	} else {
		t.app.SetFocus(t.secrets)
	}
}

func escCloses(t *tui, name string) func(*tcell.EventKey) *tcell.EventKey {
	return func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyEscape || event.Rune() == 'q' {
			t.closePage(name)
			return nil
		}
		return event
	}
}

func (t *tui) showLog() {
	t.logs.SetInputCapture(escCloses(t, "log"))
	t.page("log", t.logs, false)
	t.app.SetFocus(t.logs)
}

func (t *tui) describe() {
	view := tview.NewTextView().SetDynamicColors(true)
	view.SetBorder(true).SetBorderPadding(0, 0, 1, 1)
	k := func(s string) string { return "[#59c2ff]" + s + "[-]" }
	var sb strings.Builder
	if t.focusDests {
		i := t.selectedDest()
		if i < 0 {
			return
		}
		dest := t.config.Destinations[i]
		view.SetTitle(" [::b]describe[::-][#8a96a3](" + tview.Escape(dest.LogPrefix) + ")[-] ")
		fmt.Fprintf(&sb, "%s  %s\n%s  %s\n%s  %s\n%s  %s\n\n%s\n", k("destination:"), tview.Escape(dest.LogPrefix), k("group:      "), dest.Group,
			k("state:      "), t.destState[i], k("cmd:        "), tview.Escape(dest.Cmd), k("needs:"))
		for _, list := range []struct {
			label string
			names []string
		}{{"requires", dest.Requires}, {"wants", dest.Wants}} {
			for _, name := range list.names {
				fmt.Fprintf(&sb, "  [%s]●[-] %-32s %-9s [%s]%s[-]\n", colorName(stateColor(t.srcState[name])), name, list.label,
					colorName(stateColor(t.srcState[name])), t.srcState[name])
			}
		}
	} else {
		name := t.selectedSecret()
		if name == "" {
			return
		}
		src := t.config.Sources[name]
		view.SetTitle(" [::b]describe[::-][#8a96a3](" + name + ")[-] ")
		value := "[#8a96a3]none[-]"
		t.mu.RLock()
		if t.hasValue(name) {
			value = fmt.Sprintf("[#8a96a3]%d characters[-]", len(*src.Value))
			if src.editable() {
				value = fmt.Sprintf("[#8a96a3]%d characters (e shows it)[-]", len(*src.Value))
			}
		}
		t.mu.RUnlock()
		fmt.Fprintf(&sb, "%s         %s\n%s  %s\n%s         %s\n%s        %s\n%s        %s\n", k("name:"), name, k("description:"), tview.Escape(src.Description),
			k("kind:"), src.kind(), k("state:"), t.srcState[name], k("value:"), value)
		if src.Cmd != "" {
			fmt.Fprintf(&sb, "%s          %s\n%s      %s, %d retries\n", k("cmd:"), tview.Escape(src.Cmd), k("timeout:"), src.timeout, src.Retries)
		}
		if src.StoredIn != "" {
			fmt.Fprintf(&sb, "%s     %s\n", k("storedIn:"), tview.Escape(src.StoredIn))
		}
		fmt.Fprintf(&sb, "\n%s\n", k("destinations:"))
		for _, i := range t.destsOf(name) {
			dest := t.config.Destinations[i]
			fmt.Fprintf(&sb, "  [%s]●[-] %-40s %-8s [%s]%s[-]\n", colorName(stateColor(t.destState[i])), tview.Escape(dest.LogPrefix), dest.Group,
				colorName(stateColor(t.destState[i])), t.destState[i])
		}
	}
	text := strings.TrimRight(sb.String(), "\n")
	view.SetText(text)
	view.SetBorderColor(colorAccent)
	view.SetInputCapture(escCloses(t, "describe"))
	// A modal sized to what it says, as wide as its longest line with room for the border and padding
	width := 60
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		width = max(width, tview.TaggedStringWidth(line)+4)
	}
	t.page("describe", &modalLayer{centered(view, min(width, 150), len(lines)+2)}, true)
	t.app.SetFocus(view)
}

// modalLayer shows a modal over what is already on the screen: it dims all of that, and takes every mouse event its
// modal doesn't, so nothing behind it reacts. The keyboard goes to the modal anyway, as it has the focus
type modalLayer struct{ tview.Primitive }

func (m *modalLayer) Draw(screen tcell.Screen) {
	width, height := screen.Size()
	for y := range height {
		for x := range width {
			mainc, combc, style, _ := screen.GetContent(x, y)
			screen.SetContent(x, y, mainc, combc, style.Foreground(colorMuted).Background(colorBg).Bold(false))
		}
	}
	m.Primitive.Draw(screen)
}

func (m *modalLayer) MouseHandler() func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
	inner := m.Primitive.MouseHandler()
	return func(action tview.MouseAction, event *tcell.EventMouse, setFocus func(p tview.Primitive)) (bool, tview.Primitive) {
		_, capture := inner(action, event, setFocus)
		return true, capture
	}
}

// centered puts p in the middle of the screen at the given size
func centered(p tview.Primitive, width, height int) tview.Primitive {
	return tview.NewFlex().
		AddItem(nil, 0, 1, false).
		AddItem(tview.NewFlex().SetDirection(tview.FlexRow).
			AddItem(nil, 0, 1, false).
			AddItem(p, height, 0, true).
			AddItem(nil, 0, 1, false), width, 0, true).
		AddItem(nil, 0, 1, false)
}

func (t *tui) edit() {
	if t.focusDests {
		return
	}
	name := t.selectedSecret()
	if name == "" {
		return
	}
	src := t.config.Sources[name]
	if !src.editable() {
		t.status.SetText(fmt.Sprintf(" [#e6c446]%s is read-only: its command reads it from somewhere setup-secrets doesn't write to[-]", name))
		return
	}
	form := t.editForm(name)
	height := 9
	if src.kind() == "manual" {
		height = 11
	}
	t.page("edit", &modalLayer{centered(form, 72, height)}, true)
	t.app.SetFocus(form)
}

func (t *tui) editForm(name string) *tview.Form {
	src := t.config.Sources[name]
	manual := src.kind() == "manual"
	current := ""
	t.mu.RLock()
	if src.Value != nil {
		current = *src.Value
	}
	t.mu.RUnlock()
	form := tview.NewForm()
	form.SetFieldBackgroundColor(colorSelBg).SetButtonBackgroundColor(colorBorder).SetLabelColor(colorKey)
	form.SetBorder(true).SetBorderColor(colorAccent).SetTitle(" [::b]edit[::-][#8a96a3](" + name + ")[-] ")
	value := tview.NewInputField().SetLabel("Value").SetText(current).SetFieldWidth(44).SetMaskCharacter('•')
	repeat := tview.NewInputField().SetLabel("Repeat").SetText(current).SetFieldWidth(44).SetMaskCharacter('•')
	form.AddFormItem(value)
	if manual {
		form.AddFormItem(repeat)
	}
	form.AddCheckbox("Show", false, func(show bool) {
		mask := '•'
		if show {
			mask = 0
		}
		value.SetMaskCharacter(mask)
		repeat.SetMaskCharacter(mask)
	})
	form.AddButton("Save & store", func() {
		if manual && value.GetText() != repeat.GetText() {
			t.status.SetText(" [#f07178]The values differ[-]")
			return
		}
		if value.GetText() == "" {
			t.status.SetText(" [#f07178]An empty value isn't stored[-]")
			return
		}
		text := value.GetText()
		t.closePage("edit")
		t.do(func() { t.save(name, text) })
	})
	form.AddButton("Cancel", func() { t.closePage("edit") })
	form.SetCancelFunc(func() { t.closePage("edit") })
	return form
}

// setSrc and setDest change a state from a job
func (t *tui) setSrc(name, state string) {
	t.queue(func() { t.srcState[name] = state; t.render() })
}

func (t *tui) setDest(i int, state string) {
	t.queue(func() { t.destState[i] = state; t.render() })
}

func (t *tui) fetchOne(name string) {
	src := t.config.Sources[name]
	if src.Cmd == "" {
		t.mu.RLock()
		has := t.hasValue(name)
		t.mu.RUnlock()
		if !has {
			t.setSrc(name, stateWriteOnly)
		}
		return
	}
	t.setSrc(name, stateFetching)
	value, err := src.fetchValue(t.logs, name)
	t.mu.Lock()
	src.failed = err != nil
	if err == nil {
		src.Value = &value
	}
	t.mu.Unlock()
	switch {
	case err != nil:
		t.setSrc(name, stateFailed)
	case value == "":
		t.setSrc(name, stateMissing)
	default:
		t.setSrc(name, stateFetched)
	}
	t.refreshTokens(name)
}

// refreshTokens runs the same commands of the secrets and shows their tokens
func (t *tui) refreshTokens(names ...string) {
	tokens := t.config.sameTokens(t.logs, names)
	t.queue(func() {
		for _, name := range names {
			t.tokens[name] = tokens[name]
		}
		t.render()
	})
}

// fetchAll fetches a few at a time, so one source waiting for a workload doesn't hold up the others
func (t *tui) fetchAll() {
	var wg sync.WaitGroup
	slots := make(chan struct{}, 8)
	for _, name := range t.names {
		wg.Go(func() {
			slots <- struct{}{}
			defer func() { <-slots }()
			t.fetchOne(name)
		})
	}
	wg.Wait()
	fmt.Fprintln(t.logs, "Fetched every secret.")
}

func (t *tui) storeDest(i int) {
	dest := t.config.Destinations[i]
	t.setDest(i, stateStoring)
	t.mu.RLock()
	env, ok, fault := t.config.destinationEnv(t.logs, dest)
	t.mu.RUnlock()
	result := storeWaiting
	switch {
	case ok:
		result = runDestination(t.logs, dest, env)
	case fault:
		result = storeSkippedFault
	}
	switch result {
	case storeDone:
		t.setDest(i, stateStored)
		if err := t.config.record(t.logs, []*Destination{dest}); err != nil {
			fmt.Fprintf(t.logs, "Recording the state failed: %s\n", err)
		}
		t.refreshTokens(dest.secrets()...)
	case storeFailed:
		t.setDest(i, stateFailed)
	default:
		t.mu.RLock()
		gaps := t.fetchableGaps(dest)
		t.mu.RUnlock()
		if len(gaps) == 0 {
			t.setDest(i, stateUntouched)
		} else {
			t.setDest(i, stateWaiting)
		}
	}
}

// fetchableGaps lists the secrets the destination requires that have a command but no value
func (t *tui) fetchableGaps(dest *Destination) []string {
	var gaps []string
	for _, name := range dest.Requires {
		if src := t.config.Sources[name]; src != nil && src.Cmd != "" && !t.hasValue(name) {
			gaps = append(gaps, name)
		}
	}
	return gaps
}

// fetchAgain fetches the secrets with a command that the destinations need, but the one just saved
func (t *tui) fetchAgain(dests []int, except string) {
	var names []string
	for _, i := range dests {
		dest := t.config.Destinations[i]
		for _, name := range append(slices.Clone(dest.Requires), dest.Wants...) {
			if name != except && t.config.Sources[name] != nil && t.config.Sources[name].Cmd != "" && !slices.Contains(names, name) {
				names = append(names, name)
			}
		}
	}
	for _, name := range names {
		t.fetchOne(name)
	}
}

// save keeps the value and stores it through every destination that needs it, with the other values they need
// fetched again
func (t *tui) save(name, value string) {
	src := t.config.Sources[name]
	t.mu.Lock()
	src.Value = &value
	src.failed = false
	t.mu.Unlock()
	state := stateEdited
	if src.kind() == "manual" {
		state = stateSet
	}
	t.setSrc(name, state)
	fmt.Fprintf(t.logs, "%s: Saved, storing\n", name)
	dests := t.destsOf(name)
	t.fetchAgain(dests, name)
	for _, i := range dests {
		t.storeDest(i)
	}
}

func (t *tui) retry() {
	if t.focusDests {
		i := t.selectedDest()
		if i < 0 {
			return
		}
		t.do(func() {
			t.fetchAgain([]int{i}, "")
			t.storeDest(i)
		})
		return
	}
	name := t.selectedSecret()
	if name == "" {
		return
	}
	t.do(func() {
		t.fetchOne(name)
		dests := t.destsOf(name)
		t.fetchAgain(dests, name)
		for _, i := range dests {
			t.storeDest(i)
		}
	})
}

// dryRun marks the destinations that setup-secrets --auto --changed-only would run, and why
func (t *tui) dryRun() {
	state, err := readState(t.config.statePath)
	if err != nil {
		fmt.Fprintf(t.logs, "Reading the state failed: %s\n", err)
		return
	}
	reasons := t.config.planRun(t.logs, state)
	t.queue(func() {
		for i, dest := range t.config.Destinations {
			if reason, runs := reasons[dest]; runs {
				t.destState[i], t.reasons[i] = stateWouldRun, reason
			} else {
				t.destState[i] = stateUnchanged
				delete(t.reasons, i)
			}
		}
		t.render()
	})
}

// loadOrphans checks the outputs no destination declares any more and shows them
func (t *tui) loadOrphans() {
	orphans, err := t.config.orphans(t.logs)
	if err != nil {
		fmt.Fprintf(t.logs, "Reading the state failed: %s\n", err)
		return
	}
	t.queue(func() { t.showOrphans(orphans) })
}

func (t *tui) showOrphans(orphans []Orphan) {
	table := tview.NewTable().SetSelectable(true, false).SetFixed(1, 0)
	table.SetSelectedStyle(tcell.StyleDefault.Background(colorSelBg).Foreground(tcell.ColorWhite))
	table.SetBorder(true).SetBorderColor(colorAccent).SetBorderPadding(0, 0, 1, 1).
		SetTitle(fmt.Sprintf(" [::b]orphans[::-][#8a96a3](outputs no destination declares)[-][#ffb454][%d][-] ", len(orphans)))
	header(table, "OUTPUT", "WRITTEN BY", "WRITTEN", "STATE")
	for r, orphan := range orphans {
		state, color := "present", colorFg
		switch {
		case orphan.Err != nil:
			state, color = "can't check: "+orphan.Err.Error(), colorRed
		case !orphan.Exists:
			state, color = "already gone", colorDim
		}
		table.SetCell(r+1, 0, cell(orphan.Output, colorFg, false, false))
		table.SetCell(r+1, 1, cell(orphan.WrittenBy, colorDim, false, false))
		table.SetCell(r+1, 2, cell(orphan.At.Local().Format("2006-01-02 15:04"), colorDim, false, false))
		table.SetCell(r+1, 3, cell(state, color, false, false))
	}
	if len(orphans) == 0 {
		table.SetCell(1, 0, cell("Every output setup-secrets wrote is still declared.", colorDim, false, false).SetSelectable(false))
	}
	table.Select(1, 0)
	hint := tview.NewTextView().SetDynamicColors(true).
		SetText(" [#8a96a3]d deletes the selected output, esc goes back[-]")
	page := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(table, 0, 1, true).AddItem(hint, 1, 0, false)
	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch {
		case event.Key() == tcell.KeyEscape || event.Rune() == 'q':
			t.closePage("orphans")
			return nil
		case event.Rune() == 'd':
			row, _ := table.GetSelection()
			if row >= 1 && row <= len(orphans) && orphans[row-1].Exists {
				t.confirmDelete(orphans[row-1].Output)
			}
			return nil
		}
		return event
	})
	t.pages.RemovePage("orphans")
	t.page("orphans", page, false)
	t.app.SetFocus(table)
}

func (t *tui) confirmDelete(output string) {
	modal := tview.NewModal().
		SetText("Delete " + output + "?\n\nThere is no undo. A destination declaring it again later gets a new value only if its source can make one.").
		AddButtons([]string{"Cancel", "Delete"})
	modal.SetBackgroundColor(colorBorder).SetBorderColor(colorRed).SetTitle(" [::b]delete[::-] ")
	modal.SetDoneFunc(func(_ int, label string) {
		t.pages.RemovePage("delete")
		if label != "Delete" {
			return
		}
		t.do(func() {
			if err := t.config.removeOrphan(t.logs, output); err != nil {
				fmt.Fprintf(t.logs, "%s: Deleting failed: %s\n", output, err)
			} else {
				fmt.Fprintf(t.logs, "%s: Deleted\n", output)
			}
			t.loadOrphans()
		})
	})
	t.page("delete", &modalLayer{modal}, true)
	t.app.SetFocus(modal)
}
