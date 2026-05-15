package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/mvndaai/drop-off-pick-up/simulation"
)

// NewStatusPanel builds the right-hand status panel.
func NewStatusPanel() (fyne.CanvasObject, func([]simulation.VehicleSnapshot, []simulation.QueueEntry, []simulation.ZoneSuggestion, []simulation.DeviceControl, float32)) {
	var activeSnaps []simulation.VehicleSnapshot
	var queueSnaps []simulation.QueueEntry
	var suggestions []simulation.ZoneSuggestion
	var devices []simulation.DeviceControl

	timeLabel := widget.NewLabel("Sim Minute: 0.0")
	timeLabel.TextStyle = fyne.TextStyle{Bold: true}

	activeList := widget.NewList(
		func() int { return len(activeSnaps) },
		func() fyne.CanvasObject { return widget.NewLabel("…") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			if id >= len(activeSnaps) {
				return
			}
			s := activeSnaps[id]
			o.(*widget.Label).SetText(fmt.Sprintf("%s · %s\n%s\nPref:%s", s.LicensePlate, s.State, s.StatusMsg, s.PreferredZone))
		},
	)

	queueList := widget.NewList(
		func() int { return len(queueSnaps) },
		func() fyne.CanvasObject { return widget.NewLabel("…") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			if id >= len(queueSnaps) {
				return
			}
			q := queueSnaps[id]
			o.(*widget.Label).SetText(fmt.Sprintf("%s · %dp · %s\narr:%d min pref:%s ov:%s", q.LicensePlate, q.NumPassengers, q.DropStyle, q.ArrivalMinute, q.PreferredZone, emptyDash(q.PreferredOverride)))
		},
	)

	suggestList := widget.NewList(
		func() int { return len(suggestions) },
		func() fyne.CanvasObject { return widget.NewLabel("…") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			if id >= len(suggestions) {
				return
			}
			s := suggestions[id]
			o.(*widget.Label).SetText(fmt.Sprintf("%s -> %s (%.1fs)", s.LicensePlate, s.SuggestedZone, s.EstimatedDelay))
		},
	)

	deviceLabel := widget.NewLabel("")
	deviceLabel.Wrapping = fyne.TextWrapWord

	sections := container.NewAppTabs(
		container.NewTabItem("Active", activeList),
		container.NewTabItem("Queue", queueList),
		container.NewTabItem("Suggestions", suggestList),
		container.NewTabItem("Devices", container.NewScroll(deviceLabel)),
	)

	panel := container.NewBorder(timeLabel, nil, nil, nil, sections)

	refresh := func(snaps []simulation.VehicleSnapshot, queue []simulation.QueueEntry, suggested []simulation.ZoneSuggestion, devs []simulation.DeviceControl, simMin float32) {
		activeSnaps = snaps
		queueSnaps = queue
		suggestions = suggested
		devices = devs
		timeLabel.SetText(fmt.Sprintf("Sim Minute: %.1f", simMin))
		activeList.Refresh()
		queueList.Refresh()
		suggestList.Refresh()
		var b strings.Builder
		for _, d := range devices {
			mode := "manual"
			if d.Auto {
				mode = "auto"
			}
			state := "STOP"
			if d.Go {
				state = "GO"
			}
			b.WriteString(fmt.Sprintf("%s: %s / %s\n", d.NodeID, mode, state))
		}
		deviceLabel.SetText(b.String())
	}

	return panel, refresh
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
