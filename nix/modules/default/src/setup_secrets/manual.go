package setup_secrets

import (
	"github.com/rivo/tview"
)

func SetupManual(config *Config) error {
	app := tview.NewApplication()
	app.SetTitle("NixOS Setup Secrets").EnableMouse(true)

	go fetchSecrets(app, config)
	return app.Run()
}

func fetchSecrets(app *tview.Application, config *Config) {
	root := tview.NewGrid().SetRows(0, 1).SetColumns(5)
	root.SetBorder(true).SetTitle("Fetching secrets")
	logs := tview.NewTextView()
	logs.SetChangedFunc(func() {
		app.Draw()
		logs.ScrollToEnd()
	})
	root.AddItem(logs, 0, 0, 1, 5, 0, 0, false)
	next := tview.NewButton("Next").SetDisabled(true).SetSelectedFunc(func() {
		go editSecrets(app, config)
	})
	root.AddItem(next, 1, 3, 1, 1, 0, 0, true)

	app.SetRoot(root, true)
	config.fetch(logs)
	logs.Write([]byte("Done"))
	next.SetDisabled(false)
	app.Draw()
}

func editSecrets(app *tview.Application, config *Config) {
	root := tview.NewGrid().SetRows(0, 1).SetColumns(5)
	root.SetBorder(true).SetTitle("Fetching secrets")
	form := tview.NewForm()
	root.AddItem(form, 0, 0, 1, 5, 0, 0, false)
	save := tview.NewButton("Save")
	root.AddItem(save, 1, 3, 1, 1, 0, 0, true)
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
	save.SetSelectedFunc(func() {
		go storeSecrets(app, config)
	})
	app.SetRoot(root, true)
	app.Draw()
}

func storeSecrets(app *tview.Application, config *Config) {
	root := tview.NewGrid().SetRows(0, 1).SetColumns(5)
	root.SetBorder(true).SetTitle("Saving secrets")
	logs := tview.NewTextView()
	logs.SetChangedFunc(func() {
		app.Draw()
		logs.ScrollToEnd()
	})
	root.AddItem(logs, 0, 0, 1, 5, 0, 0, false)
	exit := tview.NewButton("Exit").SetDisabled(true).SetSelectedFunc(app.Stop)
	root.AddItem(exit, 1, 3, 1, 1, 0, 0, true)
	app.SetRoot(root, true)
	config.store(logs)
	logs.Write([]byte("Done"))
	exit.SetDisabled(false)
	app.Draw()
}
