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
}

// SerializableNode stores a route node with edge IDs for save/load.
type SerializableNode struct {
	ID         string         `json:"id"`
	Type       model.NodeType `json:"type"`
	Label      string         `json:"label"`
	X          float32        `json:"x"`
	Y          float32        `json:"y"`
	Exits      []string       `json:"exits,omitempty"`
	CrossLinks []string       `json:"crossLinks,omitempty"`
}

// ScenarioFile is the persisted simulation configuration.
type ScenarioFile struct {
	Mode            model.Mode         `json:"mode"`
	Speed           float32            `json:"speed"`
	DefaultZone     string             `json:"defaultZone"`
	Devices         []DeviceControl    `json:"devices"`
	Nodes           []SerializableNode `json:"nodes"`
	QueuedVehicles  []model.Vehicle    `json:"queuedVehicles"`
	PendingVehicles []model.Vehicle    `json:"pendingVehicles"`
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

	active  []*vehicleProgress
	queue   []*model.Vehicle
	pending []*scheduledVehicle
	devices map[string]DeviceControl
}

// NewSimulation creates a Simulation for the given mode.
func NewSimulation(mode model.Mode) *Simulation {
	route := BuildRoute(mode)
	s := &Simulation{
		Mode:          mode,
		Route:         route,
		Speed:         1,
		DefaultZoneID: "zone-a",
		devices:       map[string]DeviceControl{},
	}
	for _, n := range route.Nodes {
		if n.Type == model.NodeCrosswalk || n.Type == model.NodeSplit {
			s.devices[n.ID] = DeviceControl{NodeID: n.ID, Auto: true, Go: true}
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

// GetPedestrianSnapshots renders people walking from wait/drop zones toward crosswalk.
func (s *Simulation) GetPedestrianSnapshots() []PedestrianSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []PedestrianSnapshot{}
	for _, vp := range s.active {
		if vp.state != StateWaiting || vp.waitTotal <= 0 {
			continue
		}
		if vp.currentNode.Type != model.NodeDropZone && vp.currentNode.Type != model.NodeWaitZone {
			continue
		}
		if len(vp.currentNode.Exits) == 0 {
			continue
		}
		target := vp.currentNode.Exits[0]
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
						vp.nextNode = exits[s.splitCounter%len(exits)]
						s.splitCounter++
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

func (s *Simulation) arriveAt(vp *vehicleProgress) {
	node := vp.currentNode
	switch node.Type {
	case model.NodeIdentifier:
		vp.state = StateChecking
		vp.waitTimer = 1.5
		vp.waitTotal = 1.5
		vp.statusMsg = "Checking ID"
	case model.NodeDropZone:
		vp.state = StateWaiting
		total := float32(0)
		for _, p := range vp.vehicle.Passengers {
			total += p.EffectiveDropOffSeconds()
		}
		if total <= 0 {
			total = vp.vehicle.DropStyle.Duration()
		}
		vp.waitTimer = total
		vp.waitTotal = total
		vp.estimatedCost = total
		vp.statusMsg = fmt.Sprintf("Drop-off (%s)", vp.vehicle.DropStyle)
	case model.NodeWaitZone:
		vp.state = StateWaiting
		total := float32(3)
		for _, p := range vp.vehicle.Passengers {
			walk := p.WalkSeconds
			if walk <= 0 {
				walk = 3
			}
			total += walk
		}
		vp.waitTimer = total
		vp.waitTotal = total
		vp.estimatedCost = total
		vp.statusMsg = "Notifying passengers…"
	case model.NodeCrosswalk:
		vp.state = StateWaiting
		if !s.canProceed(node) {
			vp.waitTimer = 2
			vp.waitTotal = 2
			vp.statusMsg = "Crosswalk stop"
		} else {
			vp.waitTimer = 1
			vp.waitTotal = 1
			vp.statusMsg = "Crosswalk"
		}
	case model.NodeExit:
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
		out[i] = model.RouteNode{ID: n.ID, Type: n.Type, Label: n.Label, Pos: n.Pos}
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
			ID:    n.ID,
			Type:  n.Type,
			Label: n.Label,
			X:     n.Pos.X,
			Y:     n.Pos.Y,
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

// SuggestZoneOverrides suggests moving high-delay vehicles to zone-b.
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
				SuggestedZone:  "zone-b",
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
				SuggestedZone:  "zone-b",
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
		sn := SerializableNode{ID: n.ID, Type: n.Type, Label: n.Label, X: n.Pos.X, Y: n.Pos.Y}
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
	payload := ScenarioFile{
		Mode:            s.Mode,
		Speed:           s.Speed,
		DefaultZone:     s.DefaultZoneID,
		Devices:         devices,
		Nodes:           nodes,
		QueuedVehicles:  queued,
		PendingVehicles: pending,
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
		n := &model.RouteNode{ID: sn.ID, Type: sn.Type, Label: sn.Label, Pos: model.Point{X: sn.X, Y: sn.Y}}
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
	entry := nodeMap["street"]
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
	for _, d := range payload.Devices {
		s.devices[d.NodeID] = d
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
	street := &model.RouteNode{ID: "street", Type: model.NodeStreet, Label: "Street", Pos: model.Point{X: 350, Y: 30}}
	identifier := &model.RouteNode{ID: "identifier", Type: model.NodeIdentifier, Label: "ID Check", Pos: model.Point{X: 350, Y: 120}}
	split := &model.RouteNode{ID: "split", Type: model.NodeSplit, Label: "Split", Pos: model.Point{X: 350, Y: 210}}

	typeA := model.NodeWaitZone
	typeB := model.NodeWaitZone
	labelA := "Wait Zone A"
	labelB := "Wait Zone B"
	if mode == model.ModeDropOff {
		typeA = model.NodeDropZone
		typeB = model.NodeDropZone
		labelA = "Drop Zone A"
		labelB = "Drop Zone B"
	}

	zoneA := &model.RouteNode{ID: "zone-a", Type: typeA, Label: labelA, Pos: model.Point{X: 175, Y: 330}}
	zoneB := &model.RouteNode{ID: "zone-b", Type: typeB, Label: labelB, Pos: model.Point{X: 525, Y: 330}}
	crosswalk := &model.RouteNode{ID: "crosswalk", Type: model.NodeCrosswalk, Label: "Crosswalk", Pos: model.Point{X: 350, Y: 440}}
	joiner := &model.RouteNode{ID: "joiner", Type: model.NodeJoiner, Label: "Joiner", Pos: model.Point{X: 350, Y: 530}}
	exit := &model.RouteNode{ID: "exit", Type: model.NodeExit, Label: "Exit", Pos: model.Point{X: 350, Y: 590}}

	street.Exits = []*model.RouteNode{identifier}
	identifier.Exits = []*model.RouteNode{split}
	split.Exits = []*model.RouteNode{zoneA, zoneB}
	zoneA.Exits = []*model.RouteNode{crosswalk}
	zoneB.Exits = []*model.RouteNode{crosswalk}
	crosswalk.Exits = []*model.RouteNode{joiner}
	crosswalk.CrossLinks = []*model.RouteNode{zoneA, zoneB}
	joiner.Exits = []*model.RouteNode{exit}

	return &model.Route{Entry: street, Nodes: []*model.RouteNode{street, identifier, split, zoneA, zoneB, crosswalk, joiner, exit}}
}
