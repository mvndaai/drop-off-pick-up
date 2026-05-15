package ui

import (
	"fmt"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"

	"github.com/mvndaai/drop-off-pick-up/model"
	"github.com/mvndaai/drop-off-pick-up/simulation"
)

// NewMainWindow creates and returns the application's main window.
func NewMainWindow(a fyne.App) fyne.Window {
	w := a.NewWindow("Drop-Off / Pick-Up Emulator")
	w.Resize(fyne.NewSize(1300, 860))
	w.SetIcon(AppIcon())

	autoID := 0
	tabs := container.NewAppTabs()
	addScenario := func(mode model.Mode) {
		autoID++
		sim := simulation.NewSimulation(mode)
		title := fmt.Sprintf("%s #%d", mode.String(), autoID)
		tabs.Append(container.NewTabItem(title, buildModeTab(w, sim, title)))
	}

	addScenario(model.ModeDropOff)
	addScenario(model.ModePickUp)

	newDrop := widget.NewButton("+ Drop-Off Scenario", func() { addScenario(model.ModeDropOff) })
	newPick := widget.NewButton("+ Pick-Up Scenario", func() { addScenario(model.ModePickUp) })

	top := container.NewHBox(newDrop, newPick)
	w.SetContent(container.NewBorder(top, nil, nil, nil, tabs))
	return w
}

func buildModeTab(w fyne.Window, sim *simulation.Simulation, title string) fyne.CanvasObject {
	routeView := NewRouteView(sim)
	statusPanel, refreshStatus := NewStatusPanel()

	refreshAll := func() {
		snaps := sim.GetVehicleSnapshots()
		peds := sim.GetPedestrianSnapshots()
		suggestions := sim.SuggestZoneOverrides()
		routeView.Update(snaps, peds)
		refreshStatus(snaps, sim.GetQueueSnapshot(), suggestions, sim.GetDeviceControls(), sim.SimulationMinute())
	}
	sim.OnUpdate = refreshAll
	refreshAll()

	addVehicleBtn := widget.NewButton("➕ Add Random Vehicle", func() {
		sim.Enqueue(sim.GenerateVehicle())
		refreshAll()
	})

	setupVehicleBtn := widget.NewButton("⚙ Vehicle Setup", func() {
		showVehicleSetupDialog(w, sim, refreshAll)
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
		refreshAll()
	}

	speedLabels := []string{"0.5×", "1×", "2×", "4×", "10×"}
	speeds := []float32{0.5, 1, 2, 4, 10}
	speedSel := widget.NewSelect(speedLabels, func(sel string) {
		for i, lbl := range speedLabels {
			if lbl == sel {
				sim.SetSpeed(speeds[i])
				break
			}
		}
	})
	speedSel.SetSelected("1×")

	addNodeBtn := widget.NewButton("Add Item", func() {
		showAddNodeDialog(w, sim, refreshAll)
	})
	connectBtn := widget.NewButton("Connect Items", func() {
		showConnectDialog(w, sim, refreshAll)
	})

	saveBtn := widget.NewButton("Save", func() {
		d := dialog.NewFileSave(func(wc fyne.URIWriteCloser, err error) {
			if err != nil {
				dialog.ShowError(err, w)
				return
			}
			if wc == nil {
				return
			}
			defer wc.Close()
			if err := sim.SaveScenario(wc); err != nil {
				dialog.ShowError(err, w)
			}
		}, w)
		d.SetFileName(strings.ReplaceAll(strings.ToLower(title), " ", "-") + ".json")
		d.Show()
	})
	loadBtn := widget.NewButton("Load", func() {
		d := dialog.NewFileOpen(func(rc fyne.URIReadCloser, err error) {
			if err != nil {
				dialog.ShowError(err, w)
				return
			}
			if rc == nil {
				return
			}
			defer rc.Close()
			if err := sim.LoadScenario(rc); err != nil {
				dialog.ShowError(err, w)
				return
			}
			refreshAll()
		}, w)
		d.SetFilter(storage.NewExtensionFileFilter([]string{".json"}))
		d.Show()
	})

	suggestBtn := widget.NewButton("Suggest Overrides", func() {
		s := sim.SuggestZoneOverrides()
		if len(s) == 0 {
			dialog.ShowInformation("Overrides", "No overrides suggested right now.", w)
			return
		}
		lines := make([]string, 0, len(s))
		for _, sug := range s {
			lines = append(lines, fmt.Sprintf("%s -> %s (%.1fs)", sug.LicensePlate, sug.SuggestedZone, sug.EstimatedDelay))
		}
		dialog.ShowInformation("Overrides", strings.Join(lines, "\n"), w)
		refreshAll()
	})
	applySuggestBtn := widget.NewButton("Apply Suggestions", func() {
		for _, sug := range sim.SuggestZoneOverrides() {
			sim.ApplyOverride(sug.VehicleID, sug.SuggestedZone)
		}
		refreshAll()
	})

	deviceBtn := widget.NewButton("Control Devices", func() {
		showDeviceDialog(w, sim, refreshAll)
	})

	desc := widget.NewLabel(fmt.Sprintf("%s scenario: drag nodes to edit layout; add/connect items; save/load and run timed simulations.", sim.Mode.String()))
	desc.Wrapping = fyne.TextWrapWord

	row1 := container.NewHBox(addVehicleBtn, setupVehicleBtn, startBtn, stopBtn, widget.NewLabel("Speed"), speedSel)
	row2 := container.NewHBox(addNodeBtn, connectBtn, deviceBtn, suggestBtn, applySuggestBtn, saveBtn, loadBtn)
	controls := container.NewVBox(desc, row1, row2)

	return container.NewBorder(controls, nil, nil, statusPanel, routeView)
}

func showVehicleSetupDialog(w fyne.Window, sim *simulation.Simulation, refresh func()) {
	plate := widget.NewEntry()
	plate.SetText("CUSTOM-001")
	passengers := widget.NewEntry()
	passengers.SetText("2")
	dropBase := widget.NewEntry()
	dropBase.SetText("3")
	dropVariance := widget.NewEntry()
	dropVariance.SetText("1")
	walkTime := widget.NewEntry()
	walkTime.SetText("4")
	arrive := widget.NewEntry()
	arrive.SetText("0")
	arriveJitter := widget.NewEntry()
	arriveJitter.SetText("2")
	lateExtra := widget.NewEntry()
	lateExtra.SetText("1")

	zone := widget.NewEntry()
	zone.SetText("zone-a")
	override := widget.NewEntry()
	override.SetPlaceHolder("optional")

	styleSel := widget.NewSelect([]string{"Self", "Assisted"}, nil)
	styleSel.SetSelected("Self")

	items := []*widget.FormItem{
		widget.NewFormItem("Plate", plate),
		widget.NewFormItem("Passengers", passengers),
		widget.NewFormItem("Drop sec/person", dropBase),
		widget.NewFormItem("Drop random ±", dropVariance),
		widget.NewFormItem("Walk sec/person", walkTime),
		widget.NewFormItem("Preferred zone", zone),
		widget.NewFormItem("Override zone", override),
		widget.NewFormItem("Arrival minute", arrive),
		widget.NewFormItem("Arrival jitter ±", arriveJitter),
		widget.NewFormItem("Late extra minute", lateExtra),
		widget.NewFormItem("Drop style", styleSel),
	}

	d := dialog.NewForm("Vehicle Setup", "Add", "Cancel", items, func(ok bool) {
		if !ok {
			return
		}
		pCount, _ := strconv.Atoi(passengers.Text)
		if pCount < 1 {
			pCount = 1
		}
		if pCount > 6 {
			pCount = 6
		}
		dropBaseV, _ := strconv.ParseFloat(dropBase.Text, 32)
		dropVarV, _ := strconv.ParseFloat(dropVariance.Text, 32)
		walkV, _ := strconv.ParseFloat(walkTime.Text, 32)
		arriveV, _ := strconv.Atoi(arrive.Text)
		arriveJitV, _ := strconv.Atoi(arriveJitter.Text)
		lateV, _ := strconv.Atoi(lateExtra.Text)

		v := sim.GenerateVehicle()
		v.LicensePlate = strings.TrimSpace(plate.Text)
		v.Passengers = make([]*model.Person, pCount)
		for i := 0; i < pCount; i++ {
			v.Passengers[i] = &model.Person{
				ID:                     fmt.Sprintf("%s-P%d", v.ID, i+1),
				Name:                   fmt.Sprintf("Person %d", i+1),
				DropOffSeconds:         float32(dropBaseV),
				DropOffVarianceSeconds: float32(dropVarV),
				WalkSeconds:            float32(walkV),
			}
		}
		v.PreferredZone = strings.TrimSpace(zone.Text)
		v.PreferredZoneOverride = strings.TrimSpace(override.Text)
		v.ArrivalMinute = arriveV
		v.ArrivalJitterMinute = arriveJitV
		v.LateArrivalExtraMinute = lateV
		if styleSel.Selected == "Assisted" {
			v.DropStyle = model.DropOffAssisted
		} else {
			v.DropStyle = model.DropOffSelf
		}
		sim.Enqueue(v)
		refresh()
	}, w)
	d.Resize(fyne.NewSize(420, 540))
	d.Show()
}

func showAddNodeDialog(w fyne.Window, sim *simulation.Simulation, refresh func()) {
	typeSel := widget.NewSelect([]string{string(model.NodeStreet), string(model.NodeIdentifier), string(model.NodeSplit), string(model.NodeJoiner), string(model.NodeCrosswalk), string(model.NodeDropZone), string(model.NodeWaitZone), string(model.NodeExit)}, nil)
	typeSel.SetSelected(string(model.NodeCrosswalk))
	label := widget.NewEntry()
	label.SetText("New Item")
	x := widget.NewEntry()
	x.SetText("350")
	y := widget.NewEntry()
	y.SetText("300")

	d := dialog.NewForm("Add Item", "Add", "Cancel", []*widget.FormItem{
		widget.NewFormItem("Type", typeSel),
		widget.NewFormItem("Label", label),
		widget.NewFormItem("X", x),
		widget.NewFormItem("Y", y),
	}, func(ok bool) {
		if !ok {
			return
		}
		xv, _ := strconv.ParseFloat(x.Text, 32)
		yv, _ := strconv.ParseFloat(y.Text, 32)
		sim.AddNode(model.NodeType(typeSel.Selected), label.Text, model.Point{X: float32(xv), Y: float32(yv)})
		refresh()
	}, w)
	d.Show()
}

func showConnectDialog(w fyne.Window, sim *simulation.Simulation, refresh func()) {
	nodes := sim.RouteNodes()
	ids := make([]string, 0, len(nodes))
	for _, n := range nodes {
		ids = append(ids, n.ID)
	}
	if len(ids) < 2 {
		dialog.ShowInformation("Connect", "Need at least two items.", w)
		return
	}
	fromSel := widget.NewSelect(ids, nil)
	toSel := widget.NewSelect(ids, nil)
	fromSel.SetSelected(ids[0])
	toSel.SetSelected(ids[1])
	ped := widget.NewCheck("Pedestrian link", nil)

	d := dialog.NewForm("Connect Items", "Connect", "Cancel", []*widget.FormItem{
		widget.NewFormItem("From", fromSel),
		widget.NewFormItem("To", toSel),
		widget.NewFormItem("Type", ped),
	}, func(ok bool) {
		if !ok {
			return
		}
		sim.ConnectNodes(fromSel.Selected, toSel.Selected, ped.Checked)
		refresh()
	}, w)
	d.Show()
}

func showDeviceDialog(w fyne.Window, sim *simulation.Simulation, refresh func()) {
	devs := sim.GetDeviceControls()
	if len(devs) == 0 {
		dialog.ShowInformation("Devices", "No split/crosswalk devices available.", w)
		return
	}
	ids := make([]string, 0, len(devs))
	for _, d := range devs {
		ids = append(ids, d.NodeID)
	}
	nodeSel := widget.NewSelect(ids, nil)
	nodeSel.SetSelected(ids[0])
	autoCheck := widget.NewCheck("Auto", nil)
	autoCheck.SetChecked(true)
	goCheck := widget.NewCheck("GO (unchecked = STOP)", nil)
	goCheck.SetChecked(true)

	d := dialog.NewForm("Device Control", "Apply", "Cancel", []*widget.FormItem{
		widget.NewFormItem("Node", nodeSel),
		widget.NewFormItem("Mode", autoCheck),
		widget.NewFormItem("State", goCheck),
	}, func(ok bool) {
		if !ok {
			return
		}
		sim.SetDeviceControl(nodeSel.Selected, autoCheck.Checked, goCheck.Checked)
		refresh()
	}, w)
	d.Show()
}
