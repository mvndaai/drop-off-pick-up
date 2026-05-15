// Package simulation implements the drop-off / pick-up state machine.
package simulation

import (
	"fmt"
	"image/color"
	"math/rand"
	"sync"
	"time"

	"github.com/mvndaai/drop-off-pick-up/model"
)

// VehicleState describes what a vehicle is doing at any given moment.
type VehicleState int

const (
	StateMoving   VehicleState = iota // travelling between nodes
	StateChecking                     // waiting at the identifier check
	StateWaiting                      // stopped at a drop/wait zone or crosswalk
	StateDone                         // has exited the facility
)

func (s VehicleState) String() string {
	switch s {
	case StateMoving:
		return "Moving"
	case StateChecking:
		return "ID Check"
	case StateWaiting:
		return "Waiting"
	case StateDone:
		return "Done"
	default:
		return "Unknown"
	}
}

// vehicleProgress tracks an active vehicle's journey through the route.
type vehicleProgress struct {
	vehicle     *model.Vehicle
	currentNode *model.RouteNode
	nextNode    *model.RouteNode
	progress    float32 // 0 = at currentNode, 1 = arrived at nextNode
	state       VehicleState
	waitTimer   float32 // seconds remaining at this node
	statusMsg   string
}

// VehicleSnapshot is a read-only, UI-safe view of an active vehicle.
type VehicleSnapshot struct {
	ID            string
	LicensePlate  string
	Color         color.RGBA
	Pos           model.Point // interpolated canvas position
	State         VehicleState
	StatusMsg     string
	NumPassengers int
	NodeType      model.NodeType
}

// QueueEntry is a read-only, UI-safe view of a queued vehicle.
type QueueEntry struct {
	LicensePlate  string
	NumPassengers int
	DropStyle     string
}

// Simulation manages the end-to-end flow of vehicles through the facility.
type Simulation struct {
	mu           sync.Mutex
	Route        *model.Route
	active       []*vehicleProgress
	queue        []*model.Vehicle
	Mode         model.Mode
	Speed        float32
	running      bool
	ticker       *time.Ticker
	stop         chan struct{}
	nextID       int
	splitCounter int
	// OnUpdate is called after every simulation tick (from a goroutine).
	// Refresh UI elements inside this callback.
	OnUpdate func()
}

// NewSimulation creates a Simulation for the given operational mode.
func NewSimulation(mode model.Mode) *Simulation {
	return &Simulation{
		Mode:  mode,
		Route: BuildRoute(mode),
		Speed: 1.0,
	}
}

// Start begins the simulation loop (≈ 30 fps).
func (s *Simulation) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.stop = make(chan struct{})
	s.mu.Unlock()

	s.ticker = time.NewTicker(33 * time.Millisecond)
	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.tick(0.033)
				if s.OnUpdate != nil {
					s.OnUpdate()
				}
			case <-s.stop:
				return
			}
		}
	}()
}

// Stop halts the simulation loop.
func (s *Simulation) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	close(s.stop)
	s.ticker.Stop()
}

// IsRunning reports whether the simulation is currently active.
func (s *Simulation) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// Enqueue adds a vehicle to the waiting queue.
func (s *Simulation) Enqueue(v *model.Vehicle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = append(s.queue, v)
}

// SetSpeed changes the speed multiplier (0.5, 1, 2, 4 …).
func (s *Simulation) SetSpeed(speed float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Speed = speed
}

// GenerateVehicle creates a random sample vehicle for the current mode.
func (s *Simulation) GenerateVehicle() *model.Vehicle {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	s.mu.Unlock()

	plates := []string{"ABC-123", "XYZ-789", "LMN-456", "QRS-321", "DEF-654", "GHI-987"}
	names := []string{"Alice", "Bob", "Charlie", "Diana", "Eve", "Frank", "Grace", "Hank"}

	numPassengers := 1 + rand.Intn(3) //nolint:gosec
	passengers := make([]*model.Person, numPassengers)
	for i := range passengers {
		passengers[i] = &model.Person{
			ID:   fmt.Sprintf("P%d-%d", id, i),
			Name: names[rand.Intn(len(names))], //nolint:gosec
		}
	}

	palette := []color.RGBA{
		{R: 220, G: 50, B: 50, A: 255},
		{R: 50, G: 120, B: 220, A: 255},
		{R: 50, G: 180, B: 50, A: 255},
		{R: 200, G: 150, B: 20, A: 255},
		{R: 150, G: 50, B: 200, A: 255},
		{R: 20, G: 180, B: 180, A: 255},
	}

	dropStyle := model.DropOffSelf
	if rand.Float32() < 0.3 { //nolint:gosec
		dropStyle = model.DropOffAssisted
	}

	return &model.Vehicle{
		ID:           fmt.Sprintf("V%d", id),
		LicensePlate: plates[(id-1)%len(plates)],
		Passengers:   passengers,
		Mode:         s.Mode,
		DropStyle:    dropStyle,
		Color:        palette[(id-1)%len(palette)],
	}
}

// GetVehicleSnapshots returns a thread-safe snapshot of all active vehicles.
func (s *Simulation) GetVehicleSnapshots() []VehicleSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	snaps := make([]VehicleSnapshot, len(s.active))
	for i, vp := range s.active {
		snaps[i] = VehicleSnapshot{
			ID:            vp.vehicle.ID,
			LicensePlate:  vp.vehicle.LicensePlate,
			Color:         vp.vehicle.Color,
			Pos:           interpolatePos(vp),
			State:         vp.state,
			StatusMsg:     vp.statusMsg,
			NumPassengers: len(vp.vehicle.Passengers),
			NodeType:      vp.currentNode.Type,
		}
	}
	return snaps
}

// GetQueueSnapshot returns a thread-safe snapshot of the vehicle queue.
func (s *Simulation) GetQueueSnapshot() []QueueEntry {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries := make([]QueueEntry, len(s.queue))
	for i, v := range s.queue {
		entries[i] = QueueEntry{
			LicensePlate:  v.LicensePlate,
			NumPassengers: len(v.Passengers),
			DropStyle:     v.DropStyle.String(),
		}
	}
	return entries
}

// tick advances the simulation by dt real seconds (scaled by Speed).
func (s *Simulation) tick(dt float32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	dt *= s.Speed

	// Admit a queued vehicle if the entry node has no vehicle waiting.
	if len(s.queue) > 0 {
		entry := s.Route.Entry
		occupied := false
		for _, vp := range s.active {
			if vp.currentNode == entry && vp.state != StateDone {
				occupied = true
				break
			}
		}
		if !occupied {
			v := s.queue[0]
			s.queue = s.queue[1:]
			s.active = append(s.active, &vehicleProgress{
				vehicle:     v,
				currentNode: entry,
				progress:    0,
				state:       StateMoving,
				statusMsg:   "Entering",
			})
		}
	}

	// Update each active vehicle.
	for _, vp := range s.active {
		if vp.state != StateDone {
			s.updateVehicle(vp, dt)
		}
	}

	// Purge completed vehicles.
	alive := s.active[:0]
	for _, vp := range s.active {
		if vp.state != StateDone {
			alive = append(alive, vp)
		}
	}
	s.active = alive
}

// moveSpeed is the default vehicle movement speed in canvas pixels per second.
const moveSpeed = float32(120)

func (s *Simulation) updateVehicle(vp *vehicleProgress, dt float32) {
	switch vp.state {
	case StateMoving:
		if vp.nextNode == nil {
			exits := vp.currentNode.Exits
			switch len(exits) {
			case 0:
				vp.state = StateDone
				return
			case 1:
				vp.nextNode = exits[0]
			default:
				// Round-robin lane assignment at splits.
				vp.nextNode = exits[s.splitCounter%len(exits)]
				s.splitCounter++
			}
		}

		dist := nodeDist(vp.currentNode, vp.nextNode)
		if dist < 1 {
			dist = 1
		}
		vp.progress += moveSpeed * dt / dist
		if vp.progress >= 1.0 {
			vp.progress = 0
			vp.currentNode = vp.nextNode
			vp.nextNode = nil
			s.arriveAt(vp)
		}

	case StateChecking, StateWaiting:
		vp.waitTimer -= dt
		if vp.state == StateWaiting && vp.waitTimer > 0 {
			vp.statusMsg = fmt.Sprintf("Waiting %.1fs", vp.waitTimer)
		}
		if vp.waitTimer <= 0 {
			vp.state = StateMoving
			vp.statusMsg = "Moving"
		}
	}
}

func (s *Simulation) arriveAt(vp *vehicleProgress) {
	node := vp.currentNode
	switch node.Type {
	case model.NodeIdentifier:
		vp.state = StateChecking
		vp.waitTimer = 1.5
		vp.statusMsg = "Checking ID"

	case model.NodeDropZone:
		vp.state = StateWaiting
		vp.waitTimer = vp.vehicle.DropStyle.Duration()
		vp.statusMsg = fmt.Sprintf("Drop-off (%s)", vp.vehicle.DropStyle)

	case model.NodeWaitZone:
		// Notification + walk-out time scales with passenger count.
		vp.state = StateWaiting
		vp.waitTimer = 3.0 + float32(len(vp.vehicle.Passengers))*1.5
		vp.statusMsg = "Notifying passengers…"

	case model.NodeCrosswalk:
		// Brief stop while pedestrians clear the crossing.
		vp.state = StateWaiting
		vp.waitTimer = 1.0
		vp.statusMsg = "Crosswalk"

	case model.NodeExit:
		vp.state = StateDone

	default:
		vp.statusMsg = string(node.Type)
	}
}

// interpolatePos computes the vehicle's canvas position between two nodes.
func interpolatePos(vp *vehicleProgress) model.Point {
	if vp.nextNode == nil || vp.state != StateMoving {
		return vp.currentNode.Pos
	}
	t := vp.progress
	return model.Point{
		X: vp.currentNode.Pos.X + (vp.nextNode.Pos.X-vp.currentNode.Pos.X)*t,
		Y: vp.currentNode.Pos.Y + (vp.nextNode.Pos.Y-vp.currentNode.Pos.Y)*t,
	}
}

func nodeDist(a, b *model.RouteNode) float32 {
	dx := b.Pos.X - a.Pos.X
	dy := b.Pos.Y - a.Pos.Y
	if dx < 0 {
		dx = -dx
	}
	if dy < 0 {
		dy = -dy
	}
	return dx + dy
}

// BuildRoute constructs the node graph for the given mode.
//
// Layout (canvas coordinates, origin top-left):
//
//	Street (350, 30)
//	  │
//	ID Check (350, 120)
//	  │
//	Split (350, 210)
//	 ╱                    ╲
//	Zone A (175, 330)   Zone B (525, 330)
//	 ╲                    ╱
//	Crosswalk (350, 440)
//	  │
//	Joiner (350, 530)
//	  │
//	Exit (350, 590)
func BuildRoute(mode model.Mode) *model.Route {
	street := &model.RouteNode{
		ID: "street", Type: model.NodeStreet, Label: "Street",
		Pos: model.Point{X: 350, Y: 30},
	}
	identifier := &model.RouteNode{
		ID: "identifier", Type: model.NodeIdentifier, Label: "ID Check",
		Pos: model.Point{X: 350, Y: 120},
	}
	split := &model.RouteNode{
		ID: "split", Type: model.NodeSplit, Label: "Split",
		Pos: model.Point{X: 350, Y: 210},
	}

	var typeA, typeB model.NodeType
	var labelA, labelB string
	if mode == model.ModeDropOff {
		typeA, typeB = model.NodeDropZone, model.NodeDropZone
		labelA, labelB = "Drop Zone A", "Drop Zone B"
	} else {
		typeA, typeB = model.NodeWaitZone, model.NodeWaitZone
		labelA, labelB = "Wait Zone A", "Wait Zone B"
	}

	zoneA := &model.RouteNode{
		ID: "zone-a", Type: typeA, Label: labelA,
		Pos: model.Point{X: 175, Y: 330},
	}
	zoneB := &model.RouteNode{
		ID: "zone-b", Type: typeB, Label: labelB,
		Pos: model.Point{X: 525, Y: 330},
	}
	crosswalk := &model.RouteNode{
		ID: "crosswalk", Type: model.NodeCrosswalk, Label: "Crosswalk",
		Pos: model.Point{X: 350, Y: 440},
	}
	joiner := &model.RouteNode{
		ID: "joiner", Type: model.NodeJoiner, Label: "Joiner",
		Pos: model.Point{X: 350, Y: 530},
	}
	exit := &model.RouteNode{
		ID: "exit", Type: model.NodeExit, Label: "Exit",
		Pos: model.Point{X: 350, Y: 590},
	}

	// Wire up the route graph.
	street.Exits = []*model.RouteNode{identifier}
	identifier.Exits = []*model.RouteNode{split}
	split.Exits = []*model.RouteNode{zoneA, zoneB}
	zoneA.Exits = []*model.RouteNode{crosswalk}
	zoneB.Exits = []*model.RouteNode{crosswalk}
	crosswalk.Exits = []*model.RouteNode{joiner}
	crosswalk.CrossLinks = []*model.RouteNode{zoneA, zoneB} // pedestrian paths
	joiner.Exits = []*model.RouteNode{exit}

	return &model.Route{
		Entry: street,
		Nodes: []*model.RouteNode{street, identifier, split, zoneA, zoneB, crosswalk, joiner, exit},
	}
}
