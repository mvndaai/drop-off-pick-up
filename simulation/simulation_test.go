package simulation_test

import (
	"testing"
	"time"

	"github.com/mvndaai/drop-off-pick-up/model"
	"github.com/mvndaai/drop-off-pick-up/simulation"
)

func TestBuildRoute_DropOff(t *testing.T) {
	route := simulation.BuildRoute(model.ModeDropOff)

	if route.Entry == nil {
		t.Fatal("route.Entry must not be nil")
	}
	if route.Entry.Type != model.NodeStreet {
		t.Errorf("entry type: got %q, want %q", route.Entry.Type, model.NodeStreet)
	}
	if len(route.Nodes) == 0 {
		t.Error("route must have at least one node")
	}

	// Verify that Drop Zones exist in the node list.
	var dropZones int
	for _, n := range route.Nodes {
		if n.Type == model.NodeDropZone {
			dropZones++
		}
	}
	if dropZones < 2 {
		t.Errorf("drop-off route should have at least 2 drop zones, got %d", dropZones)
	}
}

func TestBuildRoute_PickUp(t *testing.T) {
	route := simulation.BuildRoute(model.ModePickUp)

	var waitZones int
	for _, n := range route.Nodes {
		if n.Type == model.NodeWaitZone {
			waitZones++
		}
	}
	if waitZones < 2 {
		t.Errorf("pick-up route should have at least 2 wait zones, got %d", waitZones)
	}
}

func TestBuildRoute_HasCrosswalk(t *testing.T) {
	for _, mode := range []model.Mode{model.ModeDropOff, model.ModePickUp} {
		route := simulation.BuildRoute(mode)
		var found bool
		for _, n := range route.Nodes {
			if n.Type == model.NodeCrosswalk {
				found = true
				if len(n.CrossLinks) == 0 {
					t.Errorf("crosswalk node should have cross-links in %s mode", mode)
				}
			}
		}
		if !found {
			t.Errorf("%s route missing crosswalk node", mode)
		}
	}
}

func TestGenerateVehicle(t *testing.T) {
	sim := simulation.NewSimulation(model.ModeDropOff)
	v := sim.GenerateVehicle()

	if v.ID == "" {
		t.Error("vehicle ID must not be empty")
	}
	if v.LicensePlate == "" {
		t.Error("vehicle LicensePlate must not be empty")
	}
	if len(v.Passengers) == 0 {
		t.Error("vehicle must have at least one passenger")
	}
	if v.Mode != model.ModeDropOff {
		t.Errorf("vehicle mode: got %v, want %v", v.Mode, model.ModeDropOff)
	}
}

func TestSimulation_EnqueueAndSnapshot(t *testing.T) {
	sim := simulation.NewSimulation(model.ModeDropOff)
	v := sim.GenerateVehicle()
	sim.Enqueue(v)

	queue := sim.GetQueueSnapshot()
	if len(queue) != 1 {
		t.Fatalf("queue length: got %d, want 1", len(queue))
	}
	if queue[0].LicensePlate != v.LicensePlate {
		t.Errorf("queued plate: got %q, want %q", queue[0].LicensePlate, v.LicensePlate)
	}
}

func TestSimulation_StartStop(t *testing.T) {
	sim := simulation.NewSimulation(model.ModeDropOff)

	if sim.IsRunning() {
		t.Error("simulation should not be running before Start()")
	}
	sim.Start()
	if !sim.IsRunning() {
		t.Error("simulation should be running after Start()")
	}
	sim.Stop()
	// Brief wait for goroutine to settle.
	time.Sleep(50 * time.Millisecond)
	if sim.IsRunning() {
		t.Error("simulation should not be running after Stop()")
	}
}

func TestSimulation_VehicleAdvances(t *testing.T) {
	sim := simulation.NewSimulation(model.ModeDropOff)
	sim.SetSpeed(20) // run fast

	sim.Enqueue(sim.GenerateVehicle())
	sim.Start()
	defer sim.Stop()

	// Give the simulation a moment to process.
	time.Sleep(300 * time.Millisecond)

	// The vehicle should either be active or already done.
	// Either way, the queue should now be empty.
	queue := sim.GetQueueSnapshot()
	if len(queue) != 0 {
		t.Errorf("vehicle should have been admitted from queue, but queue still has %d entries", len(queue))
	}
}

func TestDropOffStyle(t *testing.T) {
	if model.DropOffSelf.Duration() >= model.DropOffAssisted.Duration() {
		t.Error("assisted drop-off should take longer than self drop-off")
	}
	if model.DropOffSelf.String() == model.DropOffAssisted.String() {
		t.Error("drop-off style strings must be distinct")
	}
}

func TestMode_String(t *testing.T) {
	if model.ModeDropOff.String() == "" {
		t.Error("ModeDropOff.String() must not be empty")
	}
	if model.ModePickUp.String() == "" {
		t.Error("ModePickUp.String() must not be empty")
	}
	if model.ModeDropOff.String() == model.ModePickUp.String() {
		t.Error("mode strings must be distinct")
	}
}
