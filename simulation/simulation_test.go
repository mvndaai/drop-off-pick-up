package simulation_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/mvndaai/drop-off-pick-up/model"
	"github.com/mvndaai/drop-off-pick-up/simulation"
)

func TestBuildRoute_DropOff(t *testing.T) {
	route := simulation.BuildRoute(model.ModeDropOff)
	if route.Entry == nil || route.Entry.Type != model.NodeStreet {
		t.Fatal("invalid entry node")
	}
	count := 0
	for _, n := range route.Nodes {
		if n.Type == model.NodeDropZone {
			count++
		}
	}
	if count < 2 {
		t.Fatalf("expected >=2 drop zones, got %d", count)
	}
}

func TestBuildRoute_PickUp(t *testing.T) {
	route := simulation.BuildRoute(model.ModePickUp)
	count := 0
	for _, n := range route.Nodes {
		if n.Type == model.NodeWaitZone {
			count++
		}
	}
	if count < 2 {
		t.Fatalf("expected >=2 wait zones, got %d", count)
	}
}

func TestGenerateVehicle(t *testing.T) {
	sim := simulation.NewSimulation(model.ModeDropOff)
	v := sim.GenerateVehicle()
	if v.ID == "" || v.LicensePlate == "" || len(v.Passengers) == 0 {
		t.Fatal("generated vehicle missing required data")
	}
	if v.Mode != model.ModeDropOff {
		t.Fatalf("mode mismatch got %v", v.Mode)
	}
}

func TestSimulation_EnqueueAndSnapshot(t *testing.T) {
	sim := simulation.NewSimulation(model.ModeDropOff)
	v := sim.GenerateVehicle()
	v.ArrivalMinute = 0
	v.ArrivalJitterMinute = 0
	v.LateArrivalExtraMinute = 0
	sim.Enqueue(v)
	sim.Start()
	defer sim.Stop()
	time.Sleep(120 * time.Millisecond)
	q := sim.GetQueueSnapshot()
	if len(q) == 0 && len(sim.GetVehicleSnapshots()) == 0 {
		t.Fatal("expected queued or active vehicle")
	}
}

func TestSimulation_StartStop(t *testing.T) {
	sim := simulation.NewSimulation(model.ModeDropOff)
	if sim.IsRunning() {
		t.Fatal("should not be running before start")
	}
	sim.Start()
	if !sim.IsRunning() {
		t.Fatal("should be running after start")
	}
	sim.Stop()
	time.Sleep(20 * time.Millisecond)
	if sim.IsRunning() {
		t.Fatal("should not be running after stop")
	}
}

func TestSimulation_ApplyOverrideAndSuggestions(t *testing.T) {
	sim := simulation.NewSimulation(model.ModeDropOff)
	v := sim.GenerateVehicle()
	v.ArrivalMinute = 0
	v.ArrivalJitterMinute = 0
	v.LateArrivalExtraMinute = 0
	for _, p := range v.Passengers {
		p.DropOffSeconds = 6
	}
	sim.Enqueue(v)

	s := sim.SuggestZoneOverrides()
	if len(s) == 0 {
		t.Fatal("expected suggestions for long service vehicle")
	}
	if !sim.ApplyOverride(v.ID, "zone-b") {
		t.Fatal("expected override to apply")
	}
}

func TestSimulation_AddAndConnectNode(t *testing.T) {
	sim := simulation.NewSimulation(model.ModeDropOff)
	id := sim.AddNode(model.NodeCrosswalk, "Temp", model.Point{X: 100, Y: 100})
	if id == "" {
		t.Fatal("expected node id")
	}
	if !sim.ConnectNodes("zone-a", id, false) {
		t.Fatal("expected connection to succeed")
	}
	nodes := sim.RouteSnapshot()
	found := false
	for _, n := range nodes {
		if n.ID == id {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("new node missing in snapshot")
	}
}

func TestSimulation_SaveLoad(t *testing.T) {
	sim := simulation.NewSimulation(model.ModePickUp)
	v := sim.GenerateVehicle()
	v.ArrivalMinute = 0
	v.ArrivalJitterMinute = 0
	v.LateArrivalExtraMinute = 0
	sim.Enqueue(v)
	var buf bytes.Buffer
	if err := sim.SaveScenario(&buf); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	sim2 := simulation.NewSimulation(model.ModeDropOff)
	if err := sim2.LoadScenario(&buf); err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if sim2.Mode != model.ModePickUp {
		t.Fatalf("mode mismatch after load: %v", sim2.Mode)
	}
}

func TestDropOffStyle(t *testing.T) {
	if model.DropOffSelf.Duration() >= model.DropOffAssisted.Duration() {
		t.Fatal("assisted should be longer")
	}
}

func TestModeString(t *testing.T) {
	if model.ModeDropOff.String() == model.ModePickUp.String() {
		t.Fatal("mode strings should differ")
	}
}
