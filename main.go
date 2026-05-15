package main

import (
	"fyne.io/fyne/v2/app"

	"github.com/mvndaai/drop-off-pick-up/ui"
)

func main() {
	a := app.NewWithID("io.github.mvndaai.dropoffpickup")
	ui.NewMainWindow(a).ShowAndRun()
}
