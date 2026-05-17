package setup_secrets

import (
	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

func SetupManual(config *Config) error {
	app := tview.NewApplication()
	app.SetTitle("NixOS Setup Secrets").EnableMouse(true)

	go fetchSecrets(app, config)
	return app.Run()
}

func fetchSecrets(app *tview.Application, config *Config) {
	logs := tview.NewTextView()
	logs.SetChangedFunc(func() {
		app.Draw()
		logs.ScrollToEnd()
	}).SetBorder(true).SetTitle("Fetching secrets")
	app.SetRoot(logs, true)
	config.fetch(logs)
	logs.Write([]byte("Done. Press <ENTER> to continue."))
	logs.SetDoneFunc(func(key tcell.Key) {
		go editSecrets(app, config)
	})
	app.Draw()
}

func editSecrets(app *tview.Application, config *Config) {
	form := tview.NewForm()
	form.SetBorder(true).SetTitle("Fetching secrets")
	var inputFields []*tview.InputField
	for name, src := range config.Sources {
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
		go storeSecrets(app, config)
	})
	app.SetRoot(form, true)
	app.Draw()
}

func storeSecrets(app *tview.Application, config *Config) {
	logs := tview.NewTextView()
	logs.SetChangedFunc(func() {
		app.Draw()
		logs.ScrollToEnd()
	}).SetBorder(true).SetTitle("Saving secrets")
	app.SetRoot(logs, true)
	config.store(logs)
	logs.Write([]byte("Done. Press <ESC> to exit."))
	logs.SetDoneFunc(func(key tcell.Key) { app.Stop() })
	app.Draw()
}
