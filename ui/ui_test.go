package ui_test

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/mvndaai/drop-off-pick-up/model"
	"github.com/mvndaai/drop-off-pick-up/simulation"
	"github.com/mvndaai/drop-off-pick-up/ui"
)

func TestMainWindowRenders(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	w := ui.NewMainWindow(a)
	w.Resize(fyne.NewSize(1200, 800))
	w.Show()
	if w.Content() == nil {
		t.Fatal("window has no content")
	}
}

func TestRouteViewRenders(t *testing.T) {
	test.NewApp()
	for _, mode := range []model.Mode{model.ModeDropOff, model.ModePickUp} {
		sim := simulation.NewSimulation(mode)
		rv := ui.NewRouteView(sim)
		if rv == nil {
			t.Fatalf("nil RouteView for mode %v", mode)
		}
		sz := rv.MinSize()
		if sz.Width < 700 || sz.Height < 600 {
			t.Fatalf("unexpected min size %v", sz)
		}
	}
}

func TestRouteViewUpdate(t *testing.T) {
	test.NewApp()
	sim := simulation.NewSimulation(model.ModeDropOff)
	rv := ui.NewRouteView(sim)
	rv.Update(nil, nil)
	rv.Update([]simulation.VehicleSnapshot{{LicensePlate: "ABC-123", Pos: model.Point{X: 200, Y: 200}}}, []simulation.PedestrianSnapshot{{Label: "P1", Pos: model.Point{X: 210, Y: 210}}})
}

func TestIconResource(t *testing.T) {
	res := ui.AppIcon()
	if res == nil || len(res.Content()) == 0 {
		t.Fatal("icon resource missing")
	}
}
