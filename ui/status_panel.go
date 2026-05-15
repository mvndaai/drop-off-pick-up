package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/mvndaai/drop-off-pick-up/simulation"
)

// NewStatusPanel builds the right-hand status panel.
// It returns the panel widget and a refresh function that updates it with
// the latest simulation snapshots.
func NewStatusPanel() (fyne.CanvasObject, func([]simulation.VehicleSnapshot, []simulation.QueueEntry)) {
	var activeSnaps []simulation.VehicleSnapshot
	var queueSnaps []simulation.QueueEntry

	activeList := widget.NewList(
		func() int { return len(activeSnaps) },
		func() fyne.CanvasObject { return widget.NewLabel("…") },
		func(id widget.ListItemID, o fyne.CanvasObject) {
			if id >= len(activeSnaps) {
				return
			}
			s := activeSnaps[id]
			o.(*widget.Label).SetText(fmt.Sprintf(
				"%s  ·  %s\n%s",
				s.LicensePlate, s.State, s.StatusMsg,
			))
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
			o.(*widget.Label).SetText(fmt.Sprintf(
				"%s  ·  %dp  ·  %s",
				q.LicensePlate, q.NumPassengers, q.DropStyle,
			))
		},
	)

	activeHeader := widget.NewLabel("Active Vehicles")
	activeHeader.TextStyle = fyne.TextStyle{Bold: true}

	queueHeader := widget.NewLabel("Queue")
	queueHeader.TextStyle = fyne.TextStyle{Bold: true}

	panel := container.NewVSplit(
		container.NewBorder(activeHeader, nil, nil, nil, activeList),
		container.NewBorder(queueHeader, nil, nil, nil, queueList),
	)
	panel.SetOffset(0.5)

	refresh := func(snaps []simulation.VehicleSnapshot, queue []simulation.QueueEntry) {
		activeSnaps = snaps
		queueSnaps = queue
		activeList.Refresh()
		queueList.Refresh()
	}

	return panel, refresh
}
