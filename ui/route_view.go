package ui

import (
	"image/color"
	"math"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"

	"github.com/mvndaai/drop-off-pick-up/model"
	"github.com/mvndaai/drop-off-pick-up/simulation"
)

const (
	canvasW   = float32(760)
	canvasH   = float32(660)
	nodeW     = float32(110)
	nodeH     = float32(32)
	vehW      = float32(24)
	vehH      = float32(16)
	maxVehs   = 50
	maxPeds   = 120
	maxGuards = 20
)

func nodeColor(t model.NodeType) color.RGBA {
	switch t {
	case model.NodeStreet:
		return color.RGBA{R: 90, G: 90, B: 90, A: 255}
	case model.NodeQueue:
		return color.RGBA{R: 120, G: 120, B: 20, A: 255}
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
	case model.NodeServiceZone:
		return color.RGBA{R: 30, G: 160, B: 110, A: 255}
	case model.NodeBuilding:
		return color.RGBA{R: 20, G: 110, B: 60, A: 255}
	case model.NodeExit:
		return color.RGBA{R: 70, G: 70, B: 70, A: 255}
	}
	return color.RGBA{R: 120, G: 120, B: 120, A: 255}
}

type vehSprite struct {
	box   *canvas.Rectangle
	label *canvas.Text
}

type pedSprite struct {
	dot   *canvas.Circle
	label *canvas.Text
}

type guardSprite struct {
	body  *canvas.Circle    // the guard figure
	sign  *canvas.Rectangle // the stop sign
	label *canvas.Text
}

// RouteView renders route graph and supports drag/drop node editing.
type RouteView struct {
	widget.BaseWidget
	sim *simulation.Simulation

	mu          sync.Mutex
	vehicles    []simulation.VehicleSnapshot
	pedestrians []simulation.PedestrianSnapshot
	guards      []simulation.CrossingGuardSnapshot
	dragNodeID  string

	vehSprites   [maxVehs]*vehSprite
	pedSprites   [maxPeds]*pedSprite
	guardSprites [maxGuards]*guardSprite
}

func NewRouteView(sim *simulation.Simulation) *RouteView {
	rv := &RouteView{sim: sim}
	for i := range rv.vehSprites {
		box := canvas.NewRectangle(color.Transparent)
		box.CornerRadius = 3
		lbl := canvas.NewText("", color.White)
		lbl.TextSize = 8
		rv.vehSprites[i] = &vehSprite{box: box, label: lbl}
	}
	for i := range rv.pedSprites {
		dot := canvas.NewCircle(color.Transparent)
		dot.StrokeColor = color.White
		dot.StrokeWidth = 1
		lbl := canvas.NewText("", color.White)
		lbl.TextSize = 7
		rv.pedSprites[i] = &pedSprite{dot: dot, label: lbl}
	}
	for i := range rv.guardSprites {
		body := canvas.NewCircle(color.Transparent)
		body.StrokeColor = color.White
		body.StrokeWidth = 1
		sign := canvas.NewRectangle(color.Transparent)
		sign.CornerRadius = 2
		lbl := canvas.NewText("", color.White)
		lbl.TextSize = 7
		rv.guardSprites[i] = &guardSprite{body: body, sign: sign, label: lbl}
	}
	rv.ExtendBaseWidget(rv)
	return rv
}

func (rv *RouteView) Update(veh []simulation.VehicleSnapshot, peds []simulation.PedestrianSnapshot, guards []simulation.CrossingGuardSnapshot) {
	rv.mu.Lock()
	rv.vehicles = veh
	rv.pedestrians = peds
	rv.guards = guards
	rv.mu.Unlock()
	rv.Refresh()
}

func (rv *RouteView) Dragged(ev *fyne.DragEvent) {
	if rv.dragNodeID == "" {
		rv.dragNodeID = rv.hitTestNode(ev.Position)
	}
	if rv.dragNodeID == "" {
		return
	}
	rx := ev.Position.X
	ry := ev.Position.Y
	if rx < nodeW/2 {
		rx = nodeW / 2
	}
	if ry < nodeH/2 {
		ry = nodeH / 2
	}
	if rx > canvasW-nodeW/2 {
		rx = canvasW - nodeW/2
	}
	if ry > canvasH-nodeH/2 {
		ry = canvasH - nodeH/2
	}
	rv.sim.MoveNode(rv.dragNodeID, model.Point{X: rx, Y: ry})
	rv.Refresh()
}

func (rv *RouteView) DragEnd() {
	rv.dragNodeID = ""
}

func (rv *RouteView) hitTestNode(pos fyne.Position) string {
	nodes := rv.sim.RouteNodes()
	for _, n := range nodes {
		if math.Abs(float64(pos.X-n.Pos.X)) <= float64(nodeW/2) && math.Abs(float64(pos.Y-n.Pos.Y)) <= float64(nodeH/2) {
			return n.ID
		}
	}
	return ""
}

func (rv *RouteView) CreateRenderer() fyne.WidgetRenderer {
	r := &routeRenderer{view: rv}
	r.build()
	return r
}

type routeRenderer struct {
	view    *RouteView
	objects []fyne.CanvasObject
}

func (r *routeRenderer) build() {
	route := r.view.sim.RouteSnapshot()
	nodeMap := map[string]simulation.SerializableNode{}
	for _, n := range route {
		nodeMap[n.ID] = n
	}

	objs := []fyne.CanvasObject{}

	for _, node := range route {
		for _, exitID := range node.Exits {
			exit, ok := nodeMap[exitID]
			if !ok {
				continue
			}
			line := canvas.NewLine(color.RGBA{R: 70, G: 70, B: 70, A: 255})
			line.StrokeWidth = 5
			line.Position1 = fyne.NewPos(node.X, node.Y)
			line.Position2 = fyne.NewPos(exit.X, exit.Y)
			objs = append(objs, line)
		}
		for _, crossID := range node.CrossLinks {
			exit, ok := nodeMap[crossID]
			if !ok {
				continue
			}
			line := canvas.NewLine(color.RGBA{R: 160, G: 90, B: 210, A: 160})
			line.StrokeWidth = 2
			line.Position1 = fyne.NewPos(node.X, node.Y)
			line.Position2 = fyne.NewPos(exit.X, exit.Y)
			objs = append(objs, line)
		}
	}

	for _, node := range route {
		rect := canvas.NewRectangle(nodeColor(node.Type))
		rect.CornerRadius = 5
		rect.Resize(fyne.NewSize(nodeW, nodeH))
		rect.Move(fyne.NewPos(node.X-nodeW/2, node.Y-nodeH/2))
		objs = append(objs, rect)

		lbl := canvas.NewText(node.Label, color.White)
		lbl.TextSize = 11
		lbl.Alignment = fyne.TextAlignCenter
		lbl.Move(fyne.NewPos(node.X-nodeW/2, node.Y-nodeH/2+6))
		objs = append(objs, lbl)
	}

	r.view.mu.Lock()
	vehicles := append([]simulation.VehicleSnapshot(nil), r.view.vehicles...)
	pedestrians := append([]simulation.PedestrianSnapshot(nil), r.view.pedestrians...)
	guards := append([]simulation.CrossingGuardSnapshot(nil), r.view.guards...)
	r.view.mu.Unlock()

	for i, sp := range r.view.vehSprites {
		sp.box.Resize(fyne.NewSize(vehW, vehH))
		if i < len(vehicles) {
			v := vehicles[i]
			sp.box.FillColor = v.Color
			sp.box.Move(fyne.NewPos(v.Pos.X-vehW/2, v.Pos.Y-vehH/2))
			sp.label.Text = v.LicensePlate
			sp.label.Move(fyne.NewPos(v.Pos.X-vehW/2, v.Pos.Y-vehH/2-12))
		} else {
			sp.box.FillColor = color.Transparent
			sp.label.Text = ""
		}
		objs = append(objs, sp.box, sp.label)
	}

	for i, sp := range r.view.pedSprites {
		sp.dot.Resize(fyne.NewSize(8, 8))
		if i < len(pedestrians) {
			p := pedestrians[i]
			sp.dot.FillColor = color.RGBA{R: 245, G: 245, B: 245, A: 255}
			sp.dot.Move(fyne.NewPos(p.Pos.X-4, p.Pos.Y-4))
			sp.label.Text = p.Label
			sp.label.Move(fyne.NewPos(p.Pos.X+5, p.Pos.Y-8))
		} else {
			sp.dot.FillColor = color.Transparent
			sp.label.Text = ""
		}
		objs = append(objs, sp.dot, sp.label)
	}

	// Crossing guard sprites: body (circle) + stop sign (small rectangle) + label.
	// Colours signal phase: yellow = entering, red = holding, green = leaving.
	guardColors := map[simulation.CrossingGuardPhase]color.RGBA{
		simulation.GuardEntering: {R: 230, G: 200, B: 0, A: 255},
		simulation.GuardHolding:  {R: 220, G: 40, B: 40, A: 255},
		simulation.GuardLeaving:  {R: 40, G: 180, B: 40, A: 255},
	}
	for i, sp := range r.view.guardSprites {
		sp.body.Resize(fyne.NewSize(10, 10))
		sp.sign.Resize(fyne.NewSize(8, 8))
		if i < len(guards) {
			g := guards[i]
			col := guardColors[g.Phase]
			sp.body.FillColor = col
			sp.body.Move(fyne.NewPos(g.Pos.X-5, g.Pos.Y-5))
			sp.sign.FillColor = color.RGBA{R: col.R / 2, G: col.G / 2, B: col.B / 2, A: 255}
			sp.sign.Move(fyne.NewPos(g.Pos.X+6, g.Pos.Y-10))
			sp.label.Text = "🚦"
			sp.label.Move(fyne.NewPos(g.Pos.X-4, g.Pos.Y+6))
		} else {
			sp.body.FillColor = color.Transparent
			sp.sign.FillColor = color.Transparent
			sp.label.Text = ""
		}
		objs = append(objs, sp.body, sp.sign, sp.label)
	}

	r.objects = objs
}

func (r *routeRenderer) MinSize() fyne.Size { return fyne.NewSize(canvasW, canvasH) }
func (r *routeRenderer) Layout(_ fyne.Size) {}

func (r *routeRenderer) Refresh() {
	r.build()
	canvas.Refresh(r.view)
}

func (r *routeRenderer) Objects() []fyne.CanvasObject { return r.objects }
func (r *routeRenderer) Destroy()                     {}
