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
	w.Resize(fyne.NewSize(1050, 700))
	w.Show()

	if w.Content() == nil {
		t.Error("window must have content")
	}
}

func TestRouteViewRenders(t *testing.T) {
	test.NewApp()
	for _, mode := range []model.Mode{model.ModeDropOff, model.ModePickUp} {
		sim := simulation.NewSimulation(mode)
		rv := ui.NewRouteView(sim)
		if rv == nil {
			t.Errorf("NewRouteView(%v) returned nil", mode)
		}
		// MinSize must be large enough to display the route.
		sz := rv.MinSize()
		if sz.Width < 600 || sz.Height < 500 {
			t.Errorf("RouteView MinSize too small: %v", sz)
		}
	}
}

func TestRouteViewUpdate(t *testing.T) {
	test.NewApp()
	sim := simulation.NewSimulation(model.ModeDropOff)
	rv := ui.NewRouteView(sim)

	// Update with an empty snapshot – must not panic.
	rv.Update(nil)

	// Update with a sample snapshot.
	rv.Update([]simulation.VehicleSnapshot{
		{LicensePlate: "ABC-123", Pos: model.Point{X: 350, Y: 120}},
	})
}
