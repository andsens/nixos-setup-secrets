package setupsecrets

import (
	"bytes"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

// The screenshots in the README: SETUP_SECRETS_SCREENSHOTS=<dir> go test -run TestScreenshots ./...
// writes them as SVG. The values are made up.
const screenshotConfig = `{
	"sources": {
		"SONARR_API_KEY": {"description": "Sonarr API Key", "cmd": "echo 3c1f6a9e0d2b47c58e91a7f4b6d20c3e", "same": "echo 1761854520"},
		"RADARR_API_KEY": {"description": "Radarr API Key", "cmd": "echo 9a7e2c41f0b84d6e93a1c5d7e2f80b4a", "same": "echo 1761854640"},
		"PLEX_API_KEY": {"description": "Plex API Key", "cmd": "echo 'deploy/plex: 0 of 1 updated replicas available' >&2; exit 1", "retries": 2},
		"INWX_USERNAME": {"description": "INWX Username", "cmd": "echo homelab-dns", "same": "echo 4410", "storedIn": "kube cert-manager/inwx-credentials"},
		"INWX_PASSWORD": {"description": "INWX Password", "cmd": "echo vT7pQ2xLm9RkW4sE", "same": "echo 4410", "storedIn": "kube cert-manager/inwx-credentials"},
		"POSTGRESQL_PASSWORD_GRAFANA": {"description": "PostgreSQL password of grafana", "cmd": "echo Hn3kXq8ZpL2wRt6Y", "same": "echo 1187", "storedIn": "kube postgresql/grafana-password"},
		"RESTIC_DEFAULT_PASSWORD": {"description": "Encryption password for the restic default backup", "cmd": "echo correct-horse-battery-staple", "same": "echo 1763575813", "storedIn": "/etc/secrets.d/restic-default-encryption.key"},
		"ZED_PUSHOVER_TOKEN": {"description": "ZFS Event Daemon Pushover token", "cmd": "echo azGDORePK8gMaC0QOYAMyEEuzJnyUi", "storedIn": "/etc/secrets.d/pushover-credentials.env"},
		"SMB_PW_claude": {"description": "SMB Password for claude"},
		"USER_PW_anders": {"description": "Password for anders"}
	},
	"destinations": [
		{"logPrefix": "Sonarr configuration key", "group": "cluster", "requires": ["SONARR_API_KEY"], "cmd": "true"},
		{"logPrefix": "Homepage (SONARR_API_KEY)", "group": "cluster", "requires": ["SONARR_API_KEY"], "cmd": "true"},
		{"logPrefix": "Radarr configuration key", "group": "cluster", "requires": ["RADARR_API_KEY"], "cmd": "true"},
		{"logPrefix": "Homepage (RADARR_API_KEY)", "group": "cluster", "requires": ["RADARR_API_KEY"], "cmd": "true"},
		{"logPrefix": "Plex API key", "group": "cluster", "requires": ["PLEX_API_KEY"], "cmd": "true"},
		{"logPrefix": "Plex integration keys", "group": "cluster", "requires": ["PLEX_API_KEY", "RADARR_API_KEY", "SONARR_API_KEY"], "cmd": "true"},
		{"logPrefix": "Flood integration keys", "group": "cluster", "requires": ["RADARR_API_KEY", "SONARR_API_KEY"], "cmd": "true"},
		{"logPrefix": "cert-manager (INWX Credentials)", "group": "cluster", "requires": ["INWX_USERNAME", "INWX_PASSWORD"], "cmd": "true"},
		{"logPrefix": "external-dns (INWX Credentials)", "group": "cluster", "requires": ["INWX_USERNAME", "INWX_PASSWORD"], "cmd": "true"},
		{"logPrefix": "PostgreSQL password of grafana", "group": "cluster", "requires": ["POSTGRESQL_PASSWORD_GRAFANA"], "cmd": "true"},
		{"logPrefix": "Restic default backup encryption password", "group": "host", "requires": ["RESTIC_DEFAULT_PASSWORD"], "cmd": "true"},
		{"logPrefix": "ZFS Event Daemon Pushover credentials", "group": "host", "requires": ["ZED_PUSHOVER_TOKEN"], "cmd": "true"},
		{"logPrefix": "SMB Password for claude", "group": "host", "requires": ["SMB_PW_claude"], "cmd": "true"},
		{"logPrefix": "Password for anders", "group": "host", "requires": ["USER_PW_anders"], "cmd": "true"}
	]
}`

func TestScreenshots(t *testing.T) {
	dir := os.Getenv("SETUP_SECRETS_SCREENSHOTS")
	if dir == "" {
		t.Skip("SETUP_SECRETS_SCREENSHOTS is not set")
	}
	noSleep(t)

	// The TUI, after the fetch and a retry of every destination
	shot := func(name string, prepare func(t *tui) tview.Primitive) {
		config := testConfig(t, screenshotConfig)
		ui := syncTUI(config)
		ui.host = "nas"
		ui.fetchAll()
		for i := range config.Destinations {
			ui.storeDest(i)
		}
		root := prepare(ui)
		writeScreenshot(t, filepath.Join(dir, name+".svg"), root, nil, 170, 30)
	}
	shot("secrets", func(ui *tui) tview.Primitive {
		ui.secrets.Select(1+slices.Index(ui.names, "SONARR_API_KEY"), 0)
		ui.render()
		return ui.pages
	})
	shot("destinations", func(ui *tui) tview.Primitive {
		ui.focusDests = true
		ui.dests.Select(6, 0)
		ui.render()
		return ui.pages
	})
	shot("describe", func(ui *tui) tview.Primitive {
		ui.secrets.Select(1+slices.Index(ui.names, "SONARR_API_KEY"), 0)
		ui.render()
		ui.describe()
		return ui.pages
	})
	shot("edit", func(ui *tui) tview.Primitive {
		ui.secrets.Select(1+slices.Index(ui.names, "INWX_PASSWORD"), 0)
		ui.render()
		ui.edit()
		return ui.pages
	})

	// An automatic run of one group, as its unit runs it
	config := testConfig(t, screenshotConfig)
	if err := config.SelectGroups([]string{"cluster"}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	out.WriteString("[::b]# setup-secrets --auto --group cluster[::-]\n")
	var log bytes.Buffer
	err := setupAuto(config, &log, false)
	out.WriteString(tview.Escape(log.String()))
	fmt.Fprintf(&out, "[red]2026/11/19 21:04:13 ERROR %s[-]\n[::b]# echo $?[::-]\n1\n", tview.Escape(err.Error()))
	term := tview.NewTextView().SetDynamicColors(true).SetText(out.String())
	term.SetBackgroundColor(tcell.ColorDefault)
	writeScreenshot(t, filepath.Join(dir, "auto.svg"), term, nil, 100, strings.Count(out.String(), "\n")+1)
}

func writeScreenshot(t *testing.T, path string, root, focus tview.Primitive, w, h int) {
	t.Helper()
	sim := tcell.NewSimulationScreen("UTF-8")
	if err := sim.Init(); err != nil {
		t.Fatal(err)
	}
	sim.SetSize(w, h)
	if focus != nil {
		var delegate func(tview.Primitive)
		delegate = func(p tview.Primitive) { p.Focus(delegate) }
		focus.Focus(delegate)
	}
	root.SetRect(0, 0, w, h)
	root.Draw(sim)
	sim.Show()
	cells, w, h := sim.GetContents()
	if err := os.WriteFile(path, []byte(toSVG(cells, w, h)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func svgColor(c tcell.Color, def string) string {
	if c == tcell.ColorDefault || !c.Valid() {
		return def
	}
	r, g, b := c.RGB()
	return fmt.Sprintf("#%02x%02x%02x", r, g, b)
}

// toSVG draws the cells as runs of text in a fixed grid, with a background rectangle where it isn't the terminal's
func toSVG(cells []tcell.SimCell, w, h int) string {
	const cw, ch, pad, termBg, termFg = 9.0, 19.0, 14.0, "#16191d", "#d4d7dc"
	var sb strings.Builder
	fmt.Fprintf(&sb, `<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f">`+"\n", float64(w)*cw+2*pad, float64(h)*ch+2*pad)
	fmt.Fprintf(&sb, `<rect width="100%%" height="100%%" rx="8" fill="%s"/>`+"\n", termBg)
	sb.WriteString(`<g font-family="DejaVu Sans Mono, Menlo, Consolas, monospace" font-size="15">` + "\n")
	type run struct {
		x, n   int
		fg, bg string
		bold   bool
		text   strings.Builder
	}
	for y := range h {
		var runs []*run
		var cur *run
		for x := range w {
			c := cells[y*w+x]
			fgc, bgc, attr := c.Style.Decompose()
			fg, bg := svgColor(fgc, termFg), svgColor(bgc, termBg)
			if bg == "#000000" {
				bg = termBg
			}
			if attr&tcell.AttrReverse != 0 {
				fg, bg = bg, fg
			}
			bold := attr&tcell.AttrBold != 0
			r := " "
			if len(c.Runes) > 0 && c.Runes[0] != 0 {
				r = string(c.Runes)
			}
			if cur == nil || cur.fg != fg || cur.bg != bg || cur.bold != bold {
				cur = &run{x: x, fg: fg, bg: bg, bold: bold}
				runs = append(runs, cur)
			}
			cur.text.WriteString(r)
			cur.n++
		}
		for _, r := range runs {
			if r.bg != termBg {
				fmt.Fprintf(&sb, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" fill="%s"/>`+"\n",
					pad+float64(r.x)*cw, pad+float64(y)*ch, float64(r.n)*cw, ch, r.bg)
			}
		}
		for _, r := range runs {
			text := r.text.String()
			if strings.TrimSpace(text) == "" {
				continue
			}
			weight := ""
			if r.bold {
				weight = ` font-weight="bold"`
			}
			fmt.Fprintf(&sb, `<text x="%.1f" y="%.1f" fill="%s"%s textLength="%.1f" lengthAdjust="spacingAndGlyphs" xml:space="preserve">%s</text>`+"\n",
				pad+float64(r.x)*cw, pad+float64(y)*ch+ch*0.75, r.fg, weight, float64(r.n)*cw, html.EscapeString(text))
		}
	}
	sb.WriteString("</g></svg>\n")
	return sb.String()
}
