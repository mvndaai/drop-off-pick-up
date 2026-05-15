// Package ui provides the Fyne-based graphical user interface for the
// drop-off / pick-up emulator.
package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/mvndaai/drop-off-pick-up/model"
	"github.com/mvndaai/drop-off-pick-up/simulation"
)

// NewMainWindow creates and returns the application's main window.
func NewMainWindow(a fyne.App) fyne.Window {
	w := a.NewWindow("Drop-Off / Pick-Up Emulator")
	w.Resize(fyne.NewSize(1050, 780))

	dropSim := simulation.NewSimulation(model.ModeDropOff)
	pickSim := simulation.NewSimulation(model.ModePickUp)

	dropTab := buildModeTab(dropSim)
	pickTab := buildModeTab(pickSim)

	tabs := container.NewAppTabs(
		container.NewTabItem("🚌  Drop-Off", dropTab),
		container.NewTabItem("🚗  Pick-Up", pickTab),
	)
	tabs.SetTabLocation(container.TabLocationTop)

	// Stop the previous simulation when the user switches modes.
	tabs.OnChanged = func(*container.TabItem) {
		dropSim.Stop()
		pickSim.Stop()
	}

	w.SetContent(tabs)
	return w
}

// buildModeTab constructs the complete UI panel for one operational mode.
func buildModeTab(sim *simulation.Simulation) fyne.CanvasObject {
	routeView := NewRouteView(sim)
	statusPanel, refreshStatus := NewStatusPanel()

	// Wire the simulation update callback to refresh both panels.
	sim.OnUpdate = func() {
		snaps := sim.GetVehicleSnapshots()
		routeView.Update(snaps)
		refreshStatus(snaps, sim.GetQueueSnapshot())
	}

	// --- Controls ---
	addBtn := widget.NewButton("➕ Add Vehicle", func() {
		sim.Enqueue(sim.GenerateVehicle())
	})

	startBtn := widget.NewButton("▶ Start", nil)
	stopBtn := widget.NewButton("⏹ Stop", nil)
	stopBtn.Disable()

	startBtn.OnTapped = func() {
		sim.Start()
		startBtn.Disable()
		stopBtn.Enable()
	}
	stopBtn.OnTapped = func() {
		sim.Stop()
		stopBtn.Disable()
		startBtn.Enable()
	}

	speedLabels := []string{"0.5×", "1×", "2×", "4×"}
	speeds := []float32{0.5, 1.0, 2.0, 4.0}
	speedSelect := widget.NewSelect(speedLabels, func(sel string) {
		for i, lbl := range speedLabels {
			if lbl == sel {
				sim.SetSpeed(speeds[i])
				break
			}
		}
	})
	speedSelect.Selected = "1×"

	controls := container.NewHBox(
		addBtn, startBtn, stopBtn,
		widget.NewLabel("Speed:"), speedSelect,
	)

	// Mode description label
	var modeDesc string
	switch sim.Mode {
	case model.ModeDropOff:
		modeDesc = fmt.Sprintf(
			"%s mode: vehicles enter → ID check → split into lanes → drop passengers → crosswalk → exit",
			sim.Mode,
		)
	case model.ModePickUp:
		modeDesc = fmt.Sprintf(
			"%s mode: vehicles enter → ID check → split into lanes → wait for passenger notification → crosswalk → exit",
			sim.Mode,
		)
	}
	desc := widget.NewLabel(modeDesc)
	desc.Wrapping = fyne.TextWrapWord

	return container.NewBorder(
		desc,          // top: mode description
		controls,      // bottom: control buttons
		nil,           // left: (none)
		statusPanel,   // right: vehicle queue / status
		routeView,     // centre: animated route canvas
	)
}
