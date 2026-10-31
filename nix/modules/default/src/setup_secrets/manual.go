package setup_secrets

import (
	"maps"
	"slices"

	"github.com/rivo/tview"
)

func SetupManual(config *Config) error {
	app := tview.NewApplication()
	app.SetTitle("NixOS Setup Secrets").EnableMouse(true)

	root := tview.NewGrid()
	root.SetRows(0).SetColumns(0, 0).SetTitle("NixOS Setup Secrets")
	app.SetRoot(root, true)

	form := tview.NewForm()
	form.SetBorder(true).SetTitle("Loading...")
	root.AddItem(form, 0, 0, 1, 1, 0, 0, false)

	logs := tview.NewTextView()
	logs.SetChangedFunc(func() {
		app.Draw()
		logs.ScrollToEnd()
	}).SetBorder(true).SetTitle("Log")
	root.AddItem(logs, 0, 1, 1, 1, 0, 0, false)

	go func() {
		config.fetch(logs)
		logs.Write([]byte("Done.\n"))
		// Primitives may only be changed on the event loop, Form.Draw indexes items it sized earlier
		app.QueueUpdateDraw(func() { buildForm(app, form, config, logs) })
	}()
	return app.Run()
}

func buildForm(app *tview.Application, form *tview.Form, config *Config, logs *tview.TextView) {
	var inputFields []*tview.InputField
	for _, name := range slices.Sorted(maps.Keys(config.Sources)) {
		src := config.Sources[name]
		var val string
		if src.Value == nil {
			val = ""
		} else {
			val = *src.Value
		}
		f := tview.NewInputField()
		f.SetLabel(name).SetText(val).SetFieldWidth(32).
			SetMaskCharacter('*').
			SetChangedFunc(func(value string) { src.Value = &value })
		form.AddFormItem(f)
		inputFields = append(inputFields, f)
	}
	form.AddCheckbox("Show passwords", false, func(show bool) {
		for _, f := range inputFields {
			if show {
				f.SetMaskCharacter(0)
			} else {
				f.SetMaskCharacter('*')
			}
		}
	})
	form.AddButton("Save", func() {
		config.store(logs)
		logs.Write([]byte("Done.\n"))
	})
	form.SetTitle("Form")
	app.SetFocus(form)
}
