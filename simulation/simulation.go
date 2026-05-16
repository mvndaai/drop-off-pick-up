// Package simulation implements the drop-off / pick-up state machine.
package simulation

import (
	"encoding/json"
	"fmt"
	"image/color"
	"io"
	"math/rand"
	"sort"
	"sync"
	"time"

	"github.com/mvndaai/drop-off-pick-up/model"
)

// VehicleState describes what a vehicle is doing at any given moment.
type VehicleState int

const (
	StateMoving VehicleState = iota
	StateChecking
	StateWaiting
	StateDone
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

// DeviceControl models manual/automatic control devices at crosswalk and split nodes.
type DeviceControl struct {
	NodeID  string `json:"nodeId"`
	Auto    bool   `json:"auto"`
	Go      bool   `json:"go"`
	Comment string `json:"comment,omitempty"`
}

// ZoneSuggestion proposes moving a vehicle to a specific zone to reduce queue blocking.
type ZoneSuggestion struct {
	VehicleID      string  `json:"vehicleId"`
	LicensePlate   string  `json:"licensePlate"`
	SuggestedZone  string  `json:"suggestedZone"`
	EstimatedDelay float32 `json:"estimatedDelaySeconds"`
	Reason         string  `json:"reason"`
}

// SplitStrategy controls how split nodes route vehicles to exits.
type SplitStrategy string

const (
	SplitRoundRobin           SplitStrategy = "round_robin"
	SplitFillOneSide          SplitStrategy = "fill_one_side"
	SplitPreferAfterCrosswalk SplitStrategy = "prefer_after_crosswalk"
)

type vehicleProgress struct {
	vehicle       *model.Vehicle
	currentNode   *model.RouteNode
	nextNode      *model.RouteNode
	progress      float32
	state         VehicleState
	waitTimer     float32
	waitTotal     float32
	statusMsg     string
	assignedZone  string
	spawnedAt     float32
	estimatedCost float32
	idChecked     bool
}

type scheduledVehicle struct {
	vehicle       *model.Vehicle
	releaseAtSecs float32
}

// VehicleSnapshot is a read-only, UI-safe view of an active vehicle.
type VehicleSnapshot struct {
	ID            string
	LicensePlate  string
	Color         color.RGBA
	Pos           model.Point
	State         VehicleState
	StatusMsg     string
	NumPassengers int
	NodeType      model.NodeType
	PreferredZone string
}

// CrossingGuardPhase describes what the stop-sign holder is currently doing.
type CrossingGuardPhase string

const (
	GuardEntering CrossingGuardPhase = "entering"
	GuardHolding  CrossingGuardPhase = "holding"
	GuardLeaving  CrossingGuardPhase = "leaving"
)

// CrossingGuardSnapshot is a read-only, UI-safe view of an active crossing guard.
type CrossingGuardSnapshot struct {
	Pos   model.Point
	Phase CrossingGuardPhase
}

// PedestrianSnapshot represents a simulated pedestrian moving near crosswalk paths.
type PedestrianSnapshot struct {
	VehicleID string
	Pos       model.Point
	Label     string
}

// QueueEntry is a read-only, UI-safe view of a queued vehicle.
type QueueEntry struct {
	LicensePlate      string
	NumPassengers     int
	DropStyle         string
	PreferredZone     string
	PreferredOverride string
	ArrivalMinute     int
	VehicleID         string
}

// SerializableNode stores a route node with edge IDs for save/load.
type SerializableNode struct {
	ID                        string         `json:"id"`
	Type                      model.NodeType `json:"type"`
	Label                     string         `json:"label"`
	RequiresIDCheck           bool           `json:"requiresIdCheck,omitempty"`
	HasStopSignHolder         bool           `json:"hasStopSignHolder,omitempty"`
	StopSignHolderWalkSeconds float32        `json:"stopSignHolderWalkSeconds,omitempty"`
	X                         float32        `json:"x"`
	Y                         float32        `json:"y"`
	Exits                     []string       `json:"exits,omitempty"`
	CrossLinks                []string       `json:"crossLinks,omitempty"`
}

// ScenarioFile is the persisted simulation configuration.
type ScenarioFile struct {
	Mode            model.Mode               `json:"mode"`
	Speed           float32                  `json:"speed"`
	DefaultZone     string                   `json:"defaultZone"`
	Devices         []DeviceControl          `json:"devices"`
	SplitStrategies map[string]SplitStrategy `json:"splitStrategies,omitempty"`
	Nodes           []SerializableNode       `json:"nodes"`
	QueuedVehicles  []model.Vehicle          `json:"queuedVehicles"`
	PendingVehicles []model.Vehicle          `json:"pendingVehicles"`
	People          []model.Person           `json:"people,omitempty"`
}

// Simulation manages vehicles through the route.
type Simulation struct {
	mu            sync.Mutex
	Route         *model.Route
	Mode          model.Mode
	Speed         float32
	DefaultZoneID string
	OnUpdate      func()

	running      bool
	ticker       *time.Ticker
	stop         chan struct{}
	nextID       int
	splitCounter int
	simElapsed   float32
	nextNodeID   int

	active           []*vehicleProgress
	queue            []*model.Vehicle
	pending          []*scheduledVehicle
	devices          map[string]DeviceControl
	splitStrategies  map[string]SplitStrategy
	peopleByID       map[string]*model.Person
	unassignedPeople []string
	nextPersonID     int
}

// NewSimulation creates a Simulation for the given mode.
func NewSimulation(mode model.Mode) *Simulation {
	route := BuildRoute(mode)
	s := &Simulation{
		Mode:            mode,
		Route:           route,
		Speed:           1,
		DefaultZoneID:   "service-pre-a",
		devices:         map[string]DeviceControl{},
		splitStrategies: map[string]SplitStrategy{},
		peopleByID:      map[string]*model.Person{},
	}
	for _, n := range route.Nodes {
		if n.Type == model.NodeCrosswalk || n.Type == model.NodeSplit {
			s.devices[n.ID] = DeviceControl{NodeID: n.ID, Auto: true, Go: true}
		}
		if n.Type == model.NodeSplit {
			s.splitStrategies[n.ID] = SplitRoundRobin
		}
	}
	return s
}

// Start begins the simulation loop.
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

// Stop halts simulation loop.
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

func (s *Simulation) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

// SetSpeed sets simulation speed multiplier.
func (s *Simulation) SetSpeed(speed float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if speed <= 0 {
		speed = 1
	}
	s.Speed = speed
}

// SimulationMinute returns elapsed simulated minutes.
func (s *Simulation) SimulationMinute() float32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.simElapsed / 60
}

// SetDeviceControl configures split/crosswalk control devices.
func (s *Simulation) SetDeviceControl(nodeID string, auto, goFlag bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := s.devices[nodeID]
	d.NodeID = nodeID
	d.Auto = auto
	d.Go = goFlag
	s.devices[nodeID] = d
}

func (s *Simulation) GetDeviceControls() []DeviceControl {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]DeviceControl, 0, len(s.devices))
	for _, d := range s.devices {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].NodeID < out[j].NodeID })
	return out
}

func (s *Simulation) SetSplitStrategy(nodeID string, strategy SplitStrategy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strategy == "" {
		strategy = SplitRoundRobin
	}
	s.splitStrategies[nodeID] = strategy
}

func (s *Simulation) GetSplitStrategy(nodeID string) SplitStrategy {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strategy, ok := s.splitStrategies[nodeID]; ok {
		return strategy
	}
	return SplitRoundRobin
}

func (s *Simulation) SetNodeIDCheck(nodeID string, enabled bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.Route.FindNode(nodeID)
	if n == nil {
		return false
	}
	n.RequiresIDCheck = enabled
	return true
}

func (s *Simulation) SetMode(mode model.Mode) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Mode = mode
}

// GenerateVehicle creates a random vehicle using default setup values.
func (s *Simulation) GenerateVehicle() *model.Vehicle {
	s.mu.Lock()
	s.nextID++
	id := s.nextID
	defaultZone := s.DefaultZoneID
	mode := s.Mode
	s.mu.Unlock()

	names := []string{"Alice", "Bob", "Charlie", "Diana", "Eve", "Frank", "Grace", "Hank"}
	plates := []string{"ABC-123", "XYZ-789", "LMN-456", "QRS-321", "DEF-654", "GHI-987"}
	palette := []color.RGBA{
		{R: 220, G: 50, B: 50, A: 255},
		{R: 50, G: 120, B: 220, A: 255},
		{R: 50, G: 180, B: 50, A: 255},
		{R: 200, G: 150, B: 20, A: 255},
		{R: 150, G: 50, B: 200, A: 255},
		{R: 20, G: 180, B: 180, A: 255},
	}

	n := 1 + rand.Intn(3) //nolint:gosec
	passengers := make([]*model.Person, n)
	for i := 0; i < n; i++ {
		passengers[i] = &model.Person{
			ID:                     fmt.Sprintf("P%d-%d", id, i+1),
			Name:                   names[rand.Intn(len(names))], //nolint:gosec
			DropOffSeconds:         2 + rand.Float32()*2,         //nolint:gosec
			DropOffVarianceSeconds: 1,
			WalkSeconds:            3 + rand.Float32()*2, //nolint:gosec
		}
	}

	dropStyle := model.DropOffSelf
	if rand.Float32() < 0.35 { //nolint:gosec
		dropStyle = model.DropOffAssisted
	}

	return &model.Vehicle{
		ID:                     fmt.Sprintf("V%d", id),
		LicensePlate:           plates[(id-1)%len(plates)],
		Passengers:             passengers,
		Mode:                   mode,
		DropStyle:              dropStyle,
		Color:                  palette[(id-1)%len(palette)],
		PreferredZone:          defaultZone,
		ArrivalMinute:          rand.Intn(6), //nolint:gosec
		ArrivalJitterMinute:    2,
		LateArrivalExtraMinute: 1,
	}
}

type PersonEntry struct {
	ID         string
	Name       string
	AssignedTo string
}

// AddPerson adds a person to the unassigned pool.
func (s *Simulation) AddPerson(p *model.Person) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextPersonID++
	if p.ID == "" {
		p.ID = fmt.Sprintf("UP%d", s.nextPersonID)
	}
	if p.Name == "" {
		p.Name = p.ID
	}
	cp := *p
	s.peopleByID[cp.ID] = &cp
	s.unassignedPeople = append(s.unassignedPeople, cp.ID)
	return cp.ID
}

// ListPeople returns unassigned and assigned people currently known.
func (s *Simulation) ListPeople() []PersonEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PersonEntry, 0, len(s.peopleByID))
	for _, id := range s.unassignedPeople {
		if p := s.peopleByID[id]; p != nil {
			out = append(out, PersonEntry{ID: p.ID, Name: p.Name})
		}
	}
	for _, v := range s.queue {
		for _, p := range v.Passengers {
			out = append(out, PersonEntry{ID: p.ID, Name: p.Name, AssignedTo: v.ID})
		}
	}
	for _, p := range s.pending {
		for _, person := range p.vehicle.Passengers {
			out = append(out, PersonEntry{ID: person.ID, Name: person.Name, AssignedTo: p.vehicle.ID})
		}
	}
	return out
}

// PendingVehicleIDs returns IDs for assignable vehicles.
func (s *Simulation) PendingVehicleIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.queue)+len(s.pending))
	for _, v := range s.queue {
		out = append(out, v.ID)
	}
	for _, p := range s.pending {
		out = append(out, p.vehicle.ID)
	}
	sort.Strings(out)
	return out
}

// AssignPersonToVehicle moves a pooled person into a queued/pending vehicle.
func (s *Simulation) AssignPersonToVehicle(personID, vehicleID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	person := s.peopleByID[personID]
	if person == nil {
		return false
	}
	for _, v := range s.queue {
		if v.ID == vehicleID {
			cp := *person
			v.Passengers = append(v.Passengers, &cp)
			s.removeUnassigned(personID)
			return true
		}
	}
	for _, p := range s.pending {
		if p.vehicle.ID == vehicleID {
			cp := *person
			p.vehicle.Passengers = append(p.vehicle.Passengers, &cp)
			s.removeUnassigned(personID)
			return true
		}
	}
	return false
}

func (s *Simulation) removeUnassigned(personID string) {
	n := s.unassignedPeople[:0]
	for _, id := range s.unassignedPeople {
		if id != personID {
			n = append(n, id)
		}
	}
	s.unassignedPeople = n
}

// Enqueue adds a vehicle with its scheduled arrival settings.
func (s *Simulation) Enqueue(v *model.Vehicle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if v.PreferredZone == "" {
		v.PreferredZone = s.DefaultZoneID
	}
	jitter := 0
	if v.ArrivalJitterMinute > 0 {
		jitter = rand.Intn(2*v.ArrivalJitterMinute+1) - v.ArrivalJitterMinute //nolint:gosec
	}
	late := 0
	if v.LateArrivalExtraMinute > 0 && rand.Float32() < 0.25 { //nolint:gosec
		late = rand.Intn(v.LateArrivalExtraMinute + 1) //nolint:gosec
	}
	releaseMin := v.ArrivalMinute + jitter + late
	if releaseMin < 0 {
		releaseMin = 0
	}
	s.pending = append(s.pending, &scheduledVehicle{
		vehicle:       v,
		releaseAtSecs: float32(releaseMin) * 60,
	})
}

func (s *Simulation) GetQueueSnapshot() []QueueEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries := make([]QueueEntry, len(s.queue)+len(s.pending))
	i := 0
	for _, v := range s.queue {
		entries[i] = queueEntryFromVehicle(v)
		i++
	}
	for _, p := range s.pending {
		entries[i] = queueEntryFromVehicle(p.vehicle)
		i++
	}
	return entries
}

func queueEntryFromVehicle(v *model.Vehicle) QueueEntry {
	return QueueEntry{
		VehicleID:         v.ID,
		LicensePlate:      v.LicensePlate,
		NumPassengers:     len(v.Passengers),
		DropStyle:         v.DropStyle.String(),
		PreferredZone:     v.PreferredZone,
		PreferredOverride: v.PreferredZoneOverride,
		ArrivalMinute:     v.ArrivalMinute,
	}
}

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
			PreferredZone: vp.vehicle.EffectivePreferredZone(),
		}
	}
	return snaps
}

// GetPedestrianSnapshots renders people walking through service/crosswalk/building paths.
func (s *Simulation) GetPedestrianSnapshots() []PedestrianSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []PedestrianSnapshot{}
	for _, vp := range s.active {
		if vp.state != StateWaiting || vp.waitTotal <= 0 {
			continue
		}
		if vp.currentNode.Type != model.NodeDropZone &&
			vp.currentNode.Type != model.NodeWaitZone &&
			vp.currentNode.Type != model.NodeServiceZone {
			continue
		}
		target := s.firstPedTarget(vp.currentNode)
		if target == nil {
			continue
		}
		progress := 1 - (vp.waitTimer / vp.waitTotal)
		if progress < 0 {
			progress = 0
		}
		if progress > 1 {
			progress = 1
		}
		count := len(vp.vehicle.Passengers)
		if count > 3 {
			count = 3
		}
		for i := 0; i < count; i++ {
			offset := float32((i - 1) * 8)
			out = append(out, PedestrianSnapshot{
				VehicleID: vp.vehicle.ID,
				Label:     fmt.Sprintf("P%d", i+1),
				Pos: model.Point{
					X: vp.currentNode.Pos.X + (target.Pos.X-vp.currentNode.Pos.X)*progress + offset,
					Y: vp.currentNode.Pos.Y + (target.Pos.Y-vp.currentNode.Pos.Y)*progress - 10,
				},
			})
		}
	}
	return out
}

func (s *Simulation) firstPedTarget(n *model.RouteNode) *model.RouteNode {
	if len(n.CrossLinks) > 0 {
		return n.CrossLinks[0]
	}
	if len(n.Exits) > 0 {
		return n.Exits[0]
	}
	return nil
}

// GetCrossingGuardSnapshots returns active crossing-guard positions and phases.
// A guard is active whenever a vehicle is waiting at a crosswalk that has
// HasStopSignHolder enabled.  The phase sequence is:
//
//	entering → the guard walks out to stop traffic
//	holding  → stop sign is raised; pedestrians cross
//	leaving  → guard walks back; vehicle may proceed once timer expires
func (s *Simulation) GetCrossingGuardSnapshots() []CrossingGuardSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []CrossingGuardSnapshot
	for _, vp := range s.active {
		if vp.state != StateWaiting {
			continue
		}
		node := vp.currentNode
		if node.Type != model.NodeCrosswalk || !node.HasStopSignHolder {
			continue
		}
		walkTime := node.StopSignHolderWalkSeconds
		if walkTime <= 0 {
			walkTime = 3
		}
		crossingTime := float32(1)
		if !s.canProceed(node) {
			crossingTime = 2
		}
		var phase CrossingGuardPhase
		switch {
		case vp.waitTimer > crossingTime+walkTime:
			phase = GuardEntering
		case vp.waitTimer > crossingTime:
			phase = GuardHolding
		default:
			phase = GuardLeaving
		}
		out = append(out, CrossingGuardSnapshot{
			Pos:   model.Point{X: node.Pos.X + 22, Y: node.Pos.Y},
			Phase: phase,
		})
	}
	return out
}

// SetCrosswalkGuard enables or disables the stop-sign holder on a crosswalk node.
// walkSeconds is the one-way walk time; pass 0 to keep the existing value.
// Returns false if the node does not exist or is not a crosswalk.
func (s *Simulation) SetCrosswalkGuard(nodeID string, enabled bool, walkSeconds float32) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.Route.FindNode(nodeID)
	if n == nil || n.Type != model.NodeCrosswalk {
		return false
	}
	n.HasStopSignHolder = enabled
	if walkSeconds > 0 {
		n.StopSignHolderWalkSeconds = walkSeconds
	} else if n.StopSignHolderWalkSeconds <= 0 {
		n.StopSignHolderWalkSeconds = 3
	}
	return true
}

func (s *Simulation) tick(dt float32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	dt *= s.Speed
	s.simElapsed += dt

	readyPending := s.pending[:0]
	for _, p := range s.pending {
		if p.releaseAtSecs <= s.simElapsed {
			s.queue = append(s.queue, p.vehicle)
		} else {
			readyPending = append(readyPending, p)
		}
	}
	s.pending = readyPending

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
				vehicle:      v,
				currentNode:  entry,
				state:        StateMoving,
				statusMsg:    "Entering",
				assignedZone: v.EffectivePreferredZone(),
				spawnedAt:    s.simElapsed,
			})
		}
	}

	for _, vp := range s.active {
		if vp.state != StateDone {
			s.updateVehicle(vp, dt)
		}
	}

	alive := s.active[:0]
	for _, vp := range s.active {
		if vp.state != StateDone {
			alive = append(alive, vp)
		}
	}
	s.active = alive
}

const moveSpeed = float32(120)

func (s *Simulation) updateVehicle(vp *vehicleProgress, dt float32) {
	switch vp.state {
	case StateMoving:
		if vp.nextNode == nil {
			if vp.currentNode.RequiresIDCheck && !vp.idChecked {
				vp.state = StateChecking
				vp.waitTimer = 1.5
				vp.waitTotal = 1.5
				vp.statusMsg = "Checking ID"
				vp.idChecked = true
				return
			}
			exits := vp.currentNode.Exits
			switch len(exits) {
			case 0:
				vp.state = StateDone
				return
			case 1:
				vp.nextNode = exits[0]
			default:
				if vp.currentNode.Type == model.NodeSplit {
					if !s.canProceed(vp.currentNode) {
						vp.state = StateWaiting
						vp.waitTimer = 0.5
						vp.waitTotal = 0.5
						vp.statusMsg = "Split held"
						return
					}
					if preferred := s.findExitByID(exits, vp.assignedZone); preferred != nil {
						vp.nextNode = preferred
					} else {
						vp.nextNode = s.chooseSplitExit(vp.currentNode, exits)
					}
				} else {
					vp.nextNode = exits[s.splitCounter%len(exits)]
					s.splitCounter++
				}
			}
		}
		dist := nodeDist(vp.currentNode, vp.nextNode)
		if dist < 1 {
			dist = 1
		}
		vp.progress += moveSpeed * dt / dist
		if vp.progress >= 1 {
			vp.progress = 0
			vp.currentNode = vp.nextNode
			vp.nextNode = nil
			vp.idChecked = false
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

func (s *Simulation) canProceed(node *model.RouteNode) bool {
	d, ok := s.devices[node.ID]
	if !ok || d.Auto {
		return true
	}
	return d.Go
}

func (s *Simulation) findExitByID(exits []*model.RouteNode, id string) *model.RouteNode {
	for _, n := range exits {
		if n.ID == id {
			return n
		}
	}
	return nil
}

func (s *Simulation) chooseSplitExit(split *model.RouteNode, exits []*model.RouteNode) *model.RouteNode {
	strategy := s.splitStrategies[split.ID]
	switch strategy {
	case SplitFillOneSide:
		for _, ex := range exits {
			if !s.nodeOccupied(ex) {
				return ex
			}
		}
		return exits[0]
	case SplitPreferAfterCrosswalk:
		for _, ex := range exits {
			if s.reachesServiceAfterCrosswalk(ex, map[string]bool{}) && !s.nodeOccupied(ex) {
				return ex
			}
		}
		for _, ex := range exits {
			if !s.nodeOccupied(ex) {
				return ex
			}
		}
		return exits[s.splitCounter%len(exits)]
	default:
		chosen := exits[s.splitCounter%len(exits)]
		s.splitCounter++
		return chosen
	}
}

func (s *Simulation) nodeOccupied(node *model.RouteNode) bool {
	for _, vp := range s.active {
		if vp.state == StateDone {
			continue
		}
		if vp.currentNode == node || vp.nextNode == node {
			return true
		}
	}
	return false
}

func (s *Simulation) reachesServiceAfterCrosswalk(start *model.RouteNode, visited map[string]bool) bool {
	if start == nil {
		return false
	}
	if visited[start.ID] {
		return false
	}
	visited[start.ID] = true
	seenCrosswalk := start.Type == model.NodeCrosswalk
	return s.reachesServiceAfterCrosswalkWithState(start, visited, seenCrosswalk)
}

func (s *Simulation) reachesServiceAfterCrosswalkWithState(start *model.RouteNode, visited map[string]bool, seenCrosswalk bool) bool {
	for _, n := range start.Exits {
		if visited[n.ID] {
			continue
		}
		nextSeenCrosswalk := seenCrosswalk || n.Type == model.NodeCrosswalk
		if nextSeenCrosswalk && (n.Type == model.NodeServiceZone || n.Type == model.NodeDropZone || n.Type == model.NodeWaitZone) {
			return true
		}
		visited[n.ID] = true
		if s.reachesServiceAfterCrosswalkWithState(n, visited, nextSeenCrosswalk) {
			return true
		}
	}
	return false
}

func (s *Simulation) arriveAt(vp *vehicleProgress) {
	node := vp.currentNode
	switch node.Type {
	case model.NodeIdentifier:
		vp.statusMsg = "Identifier"
	case model.NodeQueue:
		vp.state = StateWaiting
		vp.waitTimer = 0.8
		vp.waitTotal = 0.8
		vp.statusMsg = "Queueing"
	case model.NodeDropZone:
		fallthrough
	case model.NodeWaitZone:
		fallthrough
	case model.NodeServiceZone:
		vp.state = StateWaiting
		total := float32(0)
		if s.Mode == model.ModePickUp {
			total = 3
			for _, p := range vp.vehicle.Passengers {
				walk := p.WalkSeconds
				if walk <= 0 {
					walk = 3
				}
				total += walk
			}
			vp.statusMsg = "Picking up passengers…"
		} else {
			for _, p := range vp.vehicle.Passengers {
				total += p.EffectiveDropOffSeconds()
			}
			if total <= 0 {
				total = vp.vehicle.DropStyle.Duration()
			}
			vp.statusMsg = fmt.Sprintf("Drop-off (%s)", vp.vehicle.DropStyle)
		}
		vp.waitTimer = total
		vp.waitTotal = total
		vp.estimatedCost = total
	case model.NodeCrosswalk:
		vp.state = StateWaiting
		baseTime := float32(1)
		statusMsg := "Crosswalk"
		if !s.canProceed(node) {
			baseTime = 2
			statusMsg = "Crosswalk stop"
		}
		if node.HasStopSignHolder {
			walkTime := node.StopSignHolderWalkSeconds
			if walkTime <= 0 {
				walkTime = 3
			}
			// Guard walks in, pedestrians cross, guard walks back out.
			total := walkTime + baseTime + walkTime
			vp.waitTimer = total
			vp.waitTotal = total
			vp.statusMsg = statusMsg + " (guard)"
		} else {
			vp.waitTimer = baseTime
			vp.waitTotal = baseTime
			vp.statusMsg = statusMsg
		}
	case model.NodeExit:
		vp.statusMsg = "Exit"
	case model.NodeBuilding:
		vp.state = StateDone
	default:
		vp.statusMsg = string(node.Type)
	}
}

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

// MoveNode supports drag/drop route editing.
func (s *Simulation) MoveNode(id string, p model.Point) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.Route.FindNode(id)
	if n == nil {
		return false
	}
	n.Pos = p
	return true
}

// AddNode adds a custom route node.
func (s *Simulation) AddNode(nodeType model.NodeType, label string, pos model.Point) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextNodeID++
	id := fmt.Sprintf("custom-%d", s.nextNodeID)
	n := &model.RouteNode{ID: id, Type: nodeType, Label: label, Pos: pos}
	s.Route.Nodes = append(s.Route.Nodes, n)
	if nodeType == model.NodeSplit || nodeType == model.NodeCrosswalk {
		s.devices[id] = DeviceControl{NodeID: id, Auto: true, Go: true}
	}
	if nodeType == model.NodeSplit {
		s.splitStrategies[id] = SplitRoundRobin
	}
	return id
}

// ConnectNodes connects two nodes either as vehicle path or crosswalk path.
func (s *Simulation) ConnectNodes(fromID, toID string, pedestrian bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	from := s.Route.FindNode(fromID)
	to := s.Route.FindNode(toID)
	if from == nil || to == nil {
		return false
	}
	if pedestrian {
		for _, n := range from.CrossLinks {
			if n == to {
				return true
			}
		}
		from.CrossLinks = append(from.CrossLinks, to)
		return true
	}
	for _, n := range from.Exits {
		if n == to {
			return true
		}
	}
	from.Exits = append(from.Exits, to)
	return true
}

func (s *Simulation) RouteNodes() []model.RouteNode {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.RouteNode, len(s.Route.Nodes))
	for i, n := range s.Route.Nodes {
		out[i] = model.RouteNode{ID: n.ID, Type: n.Type, Label: n.Label, Pos: n.Pos, RequiresIDCheck: n.RequiresIDCheck, HasStopSignHolder: n.HasStopSignHolder, StopSignHolderWalkSeconds: n.StopSignHolderWalkSeconds}
	}
	return out
}

// RouteSnapshot returns serializable route nodes including edge IDs.
func (s *Simulation) RouteSnapshot() []SerializableNode {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SerializableNode, 0, len(s.Route.Nodes))
	for _, n := range s.Route.Nodes {
		sn := SerializableNode{
			ID:                        n.ID,
			Type:                      n.Type,
			Label:                     n.Label,
			RequiresIDCheck:           n.RequiresIDCheck,
			HasStopSignHolder:         n.HasStopSignHolder,
			StopSignHolderWalkSeconds: n.StopSignHolderWalkSeconds,
			X:                         n.Pos.X,
			Y:                         n.Pos.Y,
		}
		for _, e := range n.Exits {
			sn.Exits = append(sn.Exits, e.ID)
		}
		for _, c := range n.CrossLinks {
			sn.CrossLinks = append(sn.CrossLinks, c.ID)
		}
		out = append(out, sn)
	}
	return out
}

// SuggestZoneOverrides suggests moving high-delay vehicles to alternative service lanes.
func (s *Simulation) SuggestZoneOverrides() []ZoneSuggestion {
	s.mu.Lock()
	defer s.mu.Unlock()
	suggestions := []ZoneSuggestion{}
	for _, v := range s.queue {
		est := estimateVehicleDelay(v, s.Mode)
		if est >= 10 {
			suggestions = append(suggestions, ZoneSuggestion{
				VehicleID:      v.ID,
				LicensePlate:   v.LicensePlate,
				SuggestedZone:  "service-post-a",
				EstimatedDelay: est,
				Reason:         "Long service time; move to secondary zone to keep primary lane flowing",
			})
		}
	}
	for _, p := range s.pending {
		v := p.vehicle
		est := estimateVehicleDelay(v, s.Mode)
		if est >= 10 {
			suggestions = append(suggestions, ZoneSuggestion{
				VehicleID:      v.ID,
				LicensePlate:   v.LicensePlate,
				SuggestedZone:  "service-post-a",
				EstimatedDelay: est,
				Reason:         "Long service time; pre-assign to secondary zone",
			})
		}
	}
	return suggestions
}

func estimateVehicleDelay(v *model.Vehicle, mode model.Mode) float32 {
	if mode == model.ModePickUp {
		t := float32(3)
		for _, p := range v.Passengers {
			if p.WalkSeconds > 0 {
				t += p.WalkSeconds
			} else {
				t += 3
			}
		}
		return t
	}
	t := float32(0)
	for _, p := range v.Passengers {
		t += p.DropOffSeconds
	}
	if t <= 0 {
		t = v.DropStyle.Duration()
	}
	return t
}

// ApplyOverride sets a per-vehicle preferred zone override in queue/pending sets.
func (s *Simulation) ApplyOverride(vehicleID, zoneID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.queue {
		if v.ID == vehicleID {
			v.PreferredZoneOverride = zoneID
			return true
		}
	}
	for _, p := range s.pending {
		if p.vehicle.ID == vehicleID {
			p.vehicle.PreferredZoneOverride = zoneID
			return true
		}
	}
	return false
}

// SaveScenario writes simulation route/devices/queue to JSON.
func (s *Simulation) SaveScenario(w io.Writer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	nodes := make([]SerializableNode, 0, len(s.Route.Nodes))
	for _, n := range s.Route.Nodes {
		sn := SerializableNode{ID: n.ID, Type: n.Type, Label: n.Label, RequiresIDCheck: n.RequiresIDCheck, HasStopSignHolder: n.HasStopSignHolder, StopSignHolderWalkSeconds: n.StopSignHolderWalkSeconds, X: n.Pos.X, Y: n.Pos.Y}
		for _, e := range n.Exits {
			sn.Exits = append(sn.Exits, e.ID)
		}
		for _, c := range n.CrossLinks {
			sn.CrossLinks = append(sn.CrossLinks, c.ID)
		}
		nodes = append(nodes, sn)
	}
	queued := make([]model.Vehicle, 0, len(s.queue))
	for _, v := range s.queue {
		queued = append(queued, *v)
	}
	pending := make([]model.Vehicle, 0, len(s.pending))
	for _, p := range s.pending {
		pending = append(pending, *p.vehicle)
	}
	devices := make([]DeviceControl, 0, len(s.devices))
	for _, d := range s.devices {
		devices = append(devices, d)
	}
	people := make([]model.Person, 0, len(s.peopleByID))
	for _, p := range s.peopleByID {
		people = append(people, *p)
	}
	payload := ScenarioFile{
		Mode:            s.Mode,
		Speed:           s.Speed,
		DefaultZone:     s.DefaultZoneID,
		Devices:         devices,
		SplitStrategies: s.splitStrategies,
		Nodes:           nodes,
		QueuedVehicles:  queued,
		PendingVehicles: pending,
		People:          people,
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

// LoadScenario loads route/devices/queue from JSON.
func (s *Simulation) LoadScenario(r io.Reader) error {
	var payload ScenarioFile
	if err := json.NewDecoder(r).Decode(&payload); err != nil {
		return err
	}

	nodeMap := map[string]*model.RouteNode{}
	routeNodes := make([]*model.RouteNode, 0, len(payload.Nodes))
	for _, sn := range payload.Nodes {
		n := &model.RouteNode{ID: sn.ID, Type: sn.Type, Label: sn.Label, RequiresIDCheck: sn.RequiresIDCheck, HasStopSignHolder: sn.HasStopSignHolder, StopSignHolderWalkSeconds: sn.StopSignHolderWalkSeconds, Pos: model.Point{X: sn.X, Y: sn.Y}}
		nodeMap[n.ID] = n
		routeNodes = append(routeNodes, n)
	}
	for _, sn := range payload.Nodes {
		from := nodeMap[sn.ID]
		for _, eid := range sn.Exits {
			if to := nodeMap[eid]; to != nil {
				from.Exits = append(from.Exits, to)
			}
		}
		for _, cid := range sn.CrossLinks {
			if to := nodeMap[cid]; to != nil {
				from.CrossLinks = append(from.CrossLinks, to)
			}
		}
	}
	entry := nodeMap["street-in"]
	if entry == nil && len(routeNodes) > 0 {
		entry = routeNodes[0]
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.Mode = payload.Mode
	s.Speed = payload.Speed
	if s.Speed <= 0 {
		s.Speed = 1
	}
	s.DefaultZoneID = payload.DefaultZone
	s.Route = &model.Route{Entry: entry, Nodes: routeNodes}
	s.devices = map[string]DeviceControl{}
	s.splitStrategies = map[string]SplitStrategy{}
	for nodeID, strategy := range payload.SplitStrategies {
		s.splitStrategies[nodeID] = strategy
	}
	for _, d := range payload.Devices {
		s.devices[d.NodeID] = d
	}
	s.peopleByID = map[string]*model.Person{}
	s.unassignedPeople = nil
	for i := range payload.People {
		p := payload.People[i]
		s.peopleByID[p.ID] = &p
		s.unassignedPeople = append(s.unassignedPeople, p.ID)
	}
	s.active = nil
	s.queue = nil
	s.pending = nil
	for i := range payload.QueuedVehicles {
		v := payload.QueuedVehicles[i]
		s.queue = append(s.queue, &v)
	}
	for i := range payload.PendingVehicles {
		v := payload.PendingVehicles[i]
		s.pending = append(s.pending, &scheduledVehicle{vehicle: &v, releaseAtSecs: 0})
	}
	return nil
}

// BuildRoute constructs the default node graph.
func BuildRoute(mode model.Mode) *model.Route {
	streetIn := &model.RouteNode{ID: "street-in", Type: model.NodeStreet, Label: "Street In", Pos: model.Point{X: 380, Y: 30}}
	split1 := &model.RouteNode{ID: "split-1", Type: model.NodeSplit, Label: "Split 1", Pos: model.Point{X: 380, Y: 100}}
	split2 := &model.RouteNode{ID: "split-2", Type: model.NodeSplit, Label: "Split 2", Pos: model.Point{X: 380, Y: 170}}
	queue := &model.RouteNode{ID: "queue", Type: model.NodeQueue, Label: "Queue", Pos: model.Point{X: 380, Y: 240}}
	preA := &model.RouteNode{ID: "service-pre-a", Type: model.NodeServiceZone, Label: "Service A", Pos: model.Point{X: 250, Y: 320}}
	preB := &model.RouteNode{ID: "service-pre-b", Type: model.NodeServiceZone, Label: "Service B", Pos: model.Point{X: 380, Y: 320}}
	preC := &model.RouteNode{ID: "service-pre-c", Type: model.NodeServiceZone, Label: "Service C", Pos: model.Point{X: 510, Y: 320}}
	preD := &model.RouteNode{ID: "service-pre-d", Type: model.NodeServiceZone, Label: "Service D", Pos: model.Point{X: 640, Y: 320}}
	crosswalk := &model.RouteNode{ID: "crosswalk", Type: model.NodeCrosswalk, Label: "Crosswalk", Pos: model.Point{X: 445, Y: 395}}
	postA := &model.RouteNode{ID: "service-post-a", Type: model.NodeServiceZone, Label: "Service E", Pos: model.Point{X: 250, Y: 470}}
	postB := &model.RouteNode{ID: "service-post-b", Type: model.NodeServiceZone, Label: "Service F", Pos: model.Point{X: 380, Y: 470}}
	postC := &model.RouteNode{ID: "service-post-c", Type: model.NodeServiceZone, Label: "Service G", Pos: model.Point{X: 510, Y: 470}}
	postD := &model.RouteNode{ID: "service-post-d", Type: model.NodeServiceZone, Label: "Service H", Pos: model.Point{X: 640, Y: 470}}
	joiner1 := &model.RouteNode{ID: "joiner-1", Type: model.NodeJoiner, Label: "Joiner 1", Pos: model.Point{X: 445, Y: 540}}
	joiner2 := &model.RouteNode{ID: "joiner-2", Type: model.NodeJoiner, Label: "Joiner 2", Pos: model.Point{X: 445, Y: 600}}
	streetOut := &model.RouteNode{ID: "street-out", Type: model.NodeStreet, Label: "Street Out", Pos: model.Point{X: 445, Y: 650}}
	building := &model.RouteNode{ID: "building", Type: model.NodeBuilding, Label: "Building", Pos: model.Point{X: 120, Y: 470}}

	// Car path: street > split > queue > service > joiner > joiner > street
	streetIn.Exits = []*model.RouteNode{split1}
	split1.Exits = []*model.RouteNode{split2, queue}
	split2.Exits = []*model.RouteNode{queue, preA, preB, preC, preD}
	queue.Exits = []*model.RouteNode{preA, preB, preC, preD}
	preA.Exits = []*model.RouteNode{crosswalk}
	preB.Exits = []*model.RouteNode{crosswalk}
	preC.Exits = []*model.RouteNode{crosswalk}
	preD.Exits = []*model.RouteNode{crosswalk}
	crosswalk.Exits = []*model.RouteNode{postA, postB, postC, postD}
	postA.Exits = []*model.RouteNode{joiner1}
	postB.Exits = []*model.RouteNode{joiner1}
	postC.Exits = []*model.RouteNode{joiner1}
	postD.Exits = []*model.RouteNode{joiner1}
	joiner1.Exits = []*model.RouteNode{joiner2}
	joiner2.Exits = []*model.RouteNode{streetOut}

	// Person path: entrance/exit -> service -> crosswalk -> service -> building
	streetIn.CrossLinks = []*model.RouteNode{preA, preB, preC, preD}
	preA.CrossLinks = []*model.RouteNode{crosswalk}
	preB.CrossLinks = []*model.RouteNode{crosswalk}
	preC.CrossLinks = []*model.RouteNode{crosswalk}
	preD.CrossLinks = []*model.RouteNode{crosswalk}
	crosswalk.CrossLinks = []*model.RouteNode{postA, postB, postC, postD}
	postA.CrossLinks = []*model.RouteNode{building}
	postB.CrossLinks = []*model.RouteNode{building}
	postC.CrossLinks = []*model.RouteNode{building}
	postD.CrossLinks = []*model.RouteNode{building}

	if mode == model.ModePickUp {
		split1.Label = "Split 1 (Pick-Up)"
		split2.Label = "Split 2 (Pick-Up)"
	}

	return &model.Route{
		Entry: streetIn,
		Nodes: []*model.RouteNode{
			streetIn, split1, split2, queue, preA, preB, preC, preD,
			crosswalk, postA, postB, postC, postD, joiner1, joiner2, streetOut, building,
		},
	}
}
