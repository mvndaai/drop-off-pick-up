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
	canvasW          = float32(760)
	canvasH          = float32(660)
	vehW             = float32(24)
	vehH             = float32(16)
	maxVehs          = 50
	maxPeds          = 120
	maxGuards        = 20
	defaultNodeW     = float32(110)
	defaultNodeH     = float32(32)
	roadOuterStroke  = float32(24)
	roadInnerStroke  = float32(18)
	roadCenterStroke = float32(2)
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

type routeNodeStyle struct {
	size         fyne.Size
	cornerRadius float32
	fill         color.RGBA
	stroke       color.RGBA
	label        color.Color
	diamond      bool
	crosswalk    bool
}

func styleForNodeType(t model.NodeType) routeNodeStyle {
	style := routeNodeStyle{
		size:         fyne.NewSize(defaultNodeW, defaultNodeH),
		cornerRadius: 8,
		fill:         nodeColor(t),
		stroke:       color.RGBA{R: 220, G: 220, B: 220, A: 220},
		label:        color.White,
	}
	switch t {
	case model.NodeStreet:
		style.size = fyne.NewSize(64, 124)
		style.cornerRadius = 14
	case model.NodeQueue:
		style.size = fyne.NewSize(92, 58)
		style.cornerRadius = 10
	case model.NodeIdentifier:
		style.size = fyne.NewSize(108, 42)
	case model.NodeSplit, model.NodeJoiner:
		style.size = fyne.NewSize(58, 58)
		style.diamond = true
	case model.NodeCrosswalk:
		style.size = fyne.NewSize(132, 34)
		style.cornerRadius = 4
		style.crosswalk = true
		style.label = color.Black
	case model.NodeDropZone, model.NodeWaitZone, model.NodeServiceZone:
		style.size = fyne.NewSize(120, 44)
	case model.NodeBuilding:
		style.size = fyne.NewSize(132, 70)
		style.cornerRadius = 12
	case model.NodeExit:
		style.size = fyne.NewSize(74, 42)
	}
	return style
}

func styleForSerializableNode(node simulation.SerializableNode) routeNodeStyle {
	return styleForNodeType(node.Type)
}

func centerText(text string, textColor color.Color, textSize float32, center fyne.Position) *canvas.Text {
	lbl := canvas.NewText(text, textColor)
	lbl.TextSize = textSize
	lbl.Alignment = fyne.TextAlignCenter
	size := lbl.MinSize()
	lbl.Move(fyne.NewPos(center.X-size.Width/2, center.Y-size.Height/2))
	return lbl
}

func nodeAnchor(from, to simulation.SerializableNode) fyne.Position {
	style := styleForSerializableNode(from)
	halfW := float64(style.size.Width / 2)
	halfH := float64(style.size.Height / 2)
	dx := float64(to.X - from.X)
	dy := float64(to.Y - from.Y)
	if dx == 0 && dy == 0 {
		return fyne.NewPos(from.X, from.Y)
	}
	if style.diamond {
		scale := 1 / ((math.Abs(dx) / halfW) + (math.Abs(dy) / halfH))
		return fyne.NewPos(from.X+float32(dx*scale), from.Y+float32(dy*scale))
	}
	scale := 1 / math.Max(math.Abs(dx)/halfW, math.Abs(dy)/halfH)
	return fyne.NewPos(from.X+float32(dx*scale), from.Y+float32(dy*scale))
}

func nodeBounds(nodeType model.NodeType) (float32, float32) {
	style := styleForNodeType(nodeType)
	return style.size.Width / 2, style.size.Height / 2
}

func buildNodeObjects(node simulation.SerializableNode) []fyne.CanvasObject {
	style := styleForSerializableNode(node)
	center := fyne.NewPos(node.X, node.Y)
	topLeft := fyne.NewPos(node.X-style.size.Width/2, node.Y-style.size.Height/2)
	objs := make([]fyne.CanvasObject, 0, 8)

	switch {
	case style.diamond:
		diamond := canvas.NewRasterWithPixels(func(x, y, w, h int) color.Color {
			if w == 0 || h == 0 {
				return color.Transparent
			}
			cx := float64(w-1) / 2
			cy := float64(h-1) / 2
			nx := math.Abs((float64(x) - cx) / cx)
			ny := math.Abs((float64(y) - cy) / cy)
			dist := nx + ny
			switch {
			case dist <= 0.9:
				return style.fill
			case dist <= 1.02:
				return style.stroke
			default:
				return color.Transparent
			}
		})
		diamond.Resize(style.size)
		diamond.Move(topLeft)
		objs = append(objs, diamond)
	case style.crosswalk:
		base := canvas.NewRectangle(color.RGBA{R: 63, G: 63, B: 63, A: 255})
		base.StrokeColor = style.stroke
		base.StrokeWidth = 2
		base.CornerRadius = style.cornerRadius
		base.Resize(style.size)
		base.Move(topLeft)
		objs = append(objs, base)
		stripeCount := 5
		stripeW := style.size.Width / float32(stripeCount*2)
		for i := 0; i < stripeCount; i++ {
			stripe := canvas.NewRectangle(color.RGBA{R: 240, G: 240, B: 240, A: 255})
			stripe.Resize(fyne.NewSize(stripeW, style.size.Height-10))
			stripe.Move(fyne.NewPos(topLeft.X+8+float32(i)*stripeW*2, topLeft.Y+5))
			objs = append(objs, stripe)
		}
	default:
		rect := canvas.NewRectangle(style.fill)
		rect.StrokeColor = style.stroke
		rect.StrokeWidth = 2
		rect.CornerRadius = style.cornerRadius
		rect.Resize(style.size)
		rect.Move(topLeft)
		objs = append(objs, rect)
	}

	lbl := centerText(node.Label, style.label, 10, center)
	objs = append(objs, lbl)
	return objs
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
	halfW := defaultNodeW / 2
	halfH := defaultNodeH / 2
	for _, n := range rv.sim.RouteNodes() {
		if n.ID == rv.dragNodeID {
			halfW, halfH = nodeBounds(n.Type)
			break
		}
	}
	rx := ev.Position.X
	ry := ev.Position.Y
	if rx < halfW {
		rx = halfW
	}
	if ry < halfH {
		ry = halfH
	}
	if rx > canvasW-halfW {
		rx = canvasW - halfW
	}
	if ry > canvasH-halfH {
		ry = canvasH - halfH
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
		halfW, halfH := nodeBounds(n.Type)
		if math.Abs(float64(pos.X-n.Pos.X)) <= float64(halfW) && math.Abs(float64(pos.Y-n.Pos.Y)) <= float64(halfH) {
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

	background := canvas.NewRectangle(color.RGBA{R: 42, G: 97, B: 52, A: 255})
	background.Resize(fyne.NewSize(canvasW, canvasH))
	objs := []fyne.CanvasObject{background}

	for _, node := range route {
		for _, exitID := range node.Exits {
			exit, ok := nodeMap[exitID]
			if !ok {
				continue
			}
			start := nodeAnchor(node, exit)
			end := nodeAnchor(exit, node)
			outer := canvas.NewLine(color.RGBA{R: 45, G: 47, B: 50, A: 255})
			outer.StrokeWidth = roadOuterStroke
			outer.Position1 = start
			outer.Position2 = end
			inner := canvas.NewLine(color.RGBA{R: 76, G: 79, B: 83, A: 255})
			inner.StrokeWidth = roadInnerStroke
			inner.Position1 = start
			inner.Position2 = end
			centerLine := canvas.NewLine(color.RGBA{R: 242, G: 210, B: 90, A: 220})
			centerLine.StrokeWidth = roadCenterStroke
			centerLine.Position1 = start
			centerLine.Position2 = end
			objs = append(objs, outer, inner, centerLine)
		}
		for _, crossID := range node.CrossLinks {
			exit, ok := nodeMap[crossID]
			if !ok {
				continue
			}
			start := nodeAnchor(node, exit)
			end := nodeAnchor(exit, node)
			path := canvas.NewLine(color.RGBA{R: 226, G: 226, B: 226, A: 180})
			path.StrokeWidth = 4
			path.Position1 = start
			path.Position2 = end
			objs = append(objs, path)
		}
	}

	for _, node := range route {
		objs = append(objs, buildNodeObjects(node)...)
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
