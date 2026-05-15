package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"github.com/mvndaai/drop-off-pick-up/model"
	"github.com/mvndaai/drop-off-pick-up/simulation"
)

// canvas dimensions and node/vehicle sizing constants.
const (
	canvasW = float32(700)
	canvasH = float32(620)
	nodeW   = float32(100)
	nodeH   = float32(30)
	vehW    = float32(24)
	vehH    = float32(16)
	maxVehs = 20 // pre-allocated sprite pool size
)

// nodeColor returns the fill colour for a given node type.
func nodeColor(t model.NodeType) color.RGBA {
	switch t {
	case model.NodeStreet:
		return color.RGBA{R: 90, G: 90, B: 90, A: 255}
	case model.NodeIdentifier:
		return color.RGBA{R: 210, G: 120, B: 10, A: 255}
	case model.NodeSplit:
		return color.RGBA{R: 40, G: 90, B: 200, A: 255}
	case model.NodeJoiner:
		return color.RGBA{R: 40, G: 90, B: 200, A: 255}
	case model.NodeCrosswalk:
		return color.RGBA{R: 130, G: 40, B: 190, A: 255}
	case model.NodeDropZone:
		return color.RGBA{R: 30, G: 150, B: 50, A: 255}
	case model.NodeWaitZone:
		return color.RGBA{R: 180, G: 150, B: 10, A: 255}
	case model.NodeExit:
		return color.RGBA{R: 70, G: 70, B: 70, A: 255}
	}
	return color.RGBA{R: 100, G: 100, B: 100, A: 255}
}

// vehSprite holds the canvas objects for one vehicle slot.
type vehSprite struct {
	box   *canvas.Rectangle
	label *canvas.Text
}

// RouteView is a Fyne widget that renders the route graph and animates
// vehicles as they move through it.
type RouteView struct {
	widget.BaseWidget
	sim     *simulation.Simulation
	sprites [maxVehs]*vehSprite
}

// NewRouteView creates a RouteView bound to the given simulation.
func NewRouteView(sim *simulation.Simulation) *RouteView {
	rv := &RouteView{sim: sim}
	for i := range rv.sprites {
		box := canvas.NewRectangle(color.Transparent)
		box.CornerRadius = 3

		lbl := canvas.NewText("", color.White)
		lbl.TextSize = 8

		rv.sprites[i] = &vehSprite{box: box, label: lbl}
	}
	rv.ExtendBaseWidget(rv)
	return rv
}

// Update refreshes all vehicle sprite positions from a snapshot slice.
// Safe to call from a goroutine.
func (rv *RouteView) Update(snaps []simulation.VehicleSnapshot) {
	// Hide every sprite first.
	for _, sp := range rv.sprites {
		if sp.box.FillColor != (color.RGBA{}) {
			sp.box.FillColor = color.Transparent
			sp.label.Text = ""
		}
	}
	// Re-position sprites that have an active vehicle.
	for i, snap := range snaps {
		if i >= maxVehs {
			break
		}
		sp := rv.sprites[i]
		sp.box.FillColor = snap.Color
		sp.box.Move(fyne.NewPos(snap.Pos.X-vehW/2, snap.Pos.Y-vehH/2))
		sp.label.Text = snap.LicensePlate
		sp.label.Move(fyne.NewPos(snap.Pos.X-vehW/2, snap.Pos.Y-vehH/2-13))
	}
	rv.Refresh()
}

// CreateRenderer implements fyne.Widget.
func (rv *RouteView) CreateRenderer() fyne.WidgetRenderer {
	r := &routeRenderer{view: rv}
	r.build()
	return r
}

// routeRenderer draws the static route graph plus the animated vehicle sprites.
type routeRenderer struct {
	view    *RouteView
	objects []fyne.CanvasObject
}

func (r *routeRenderer) build() {
	objs := []fyne.CanvasObject{}

	// Draw route connection lines.
	for _, node := range r.view.sim.Route.Nodes {
		for _, exit := range node.Exits {
			line := canvas.NewLine(color.RGBA{R: 70, G: 70, B: 70, A: 255})
			line.StrokeWidth = 5
			line.Position1 = fyne.NewPos(node.Pos.X, node.Pos.Y)
			line.Position2 = fyne.NewPos(exit.Pos.X, exit.Pos.Y)
			objs = append(objs, line)
		}
		// Dashed-style crosswalk pedestrian links.
		for _, cl := range node.CrossLinks {
			line := canvas.NewLine(color.RGBA{R: 160, G: 90, B: 210, A: 160})
			line.StrokeWidth = 2
			line.Position1 = fyne.NewPos(node.Pos.X, node.Pos.Y)
			line.Position2 = fyne.NewPos(cl.Pos.X, cl.Pos.Y)
			objs = append(objs, line)
		}
	}

	// Draw node boxes (background rectangle + label text).
	for _, node := range r.view.sim.Route.Nodes {
		rect := canvas.NewRectangle(nodeColor(node.Type))
		rect.CornerRadius = 5
		rect.Resize(fyne.NewSize(nodeW, nodeH))
		rect.Move(fyne.NewPos(node.Pos.X-nodeW/2, node.Pos.Y-nodeH/2))
		objs = append(objs, rect)

		lbl := canvas.NewText(node.Label, color.White)
		lbl.TextSize = 11
		lbl.Alignment = fyne.TextAlignCenter
		// Centre the text inside the box.
		lbl.Move(fyne.NewPos(node.Pos.X-nodeW/2, node.Pos.Y-nodeH/2+5))
		objs = append(objs, lbl)
	}

	// Add pre-allocated vehicle sprites (initially invisible).
	for _, sp := range r.view.sprites {
		sp.box.Resize(fyne.NewSize(vehW, vehH))
		objs = append(objs, sp.box, sp.label)
	}

	r.objects = objs
}

// MinSize returns the minimum canvas size needed to display the full route.
func (r *routeRenderer) MinSize() fyne.Size {
	return fyne.NewSize(canvasW, canvasH)
}

func (r *routeRenderer) Layout(_ fyne.Size) {}

func (r *routeRenderer) Refresh() {
	canvas.Refresh(r.view)
}

func (r *routeRenderer) Objects() []fyne.CanvasObject {
	return r.objects
}

func (r *routeRenderer) Destroy() {}
