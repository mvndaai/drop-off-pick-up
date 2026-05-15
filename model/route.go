package model

// Point is a 2-D canvas coordinate used to position route nodes.
type Point struct {
	X, Y float32
}

// NodeType classifies each node by its role in the traffic flow.
type NodeType string

const (
	// NodeStreet is the entry point from the public road.
	NodeStreet NodeType = "Street"
	// NodeIdentifier is a license-plate / ID check station.
	// It can sit on the main route or accept walk-up pedestrians who
	// then exit via a crosswalk.
	NodeIdentifier NodeType = "Identifier"
	// NodeSplit divides traffic into two or more lanes.
	NodeSplit NodeType = "Split"
	// NodeJoiner merges multiple lanes back into one.
	NodeJoiner NodeType = "Joiner"
	// NodeCrosswalk allows pedestrians to cross between paths or
	// reach a waiting area / exit.  It can be automated, staffed,
	// or both; a staff member can initiate the crossing manually.
	NodeCrosswalk NodeType = "Crosswalk"
	// NodeDropZone is where passengers exit the vehicle (drop-off mode).
	NodeDropZone NodeType = "DropZone"
	// NodeWaitZone is where a vehicle parks while a passenger is
	// notified and walks out (pick-up mode).
	NodeWaitZone NodeType = "WaitZone"
	// NodeExit is where a vehicle leaves the facility.
	NodeExit NodeType = "Exit"
)

// RouteNode is a single vertex in the route graph.
type RouteNode struct {
	ID   string
	Type NodeType
	// Label is the human-readable name shown in the UI.
	Label string
	// Pos is the canvas position of this node's centre.
	Pos Point
	// Exits lists the nodes a vehicle may travel to next.
	// More than one exit means the node acts as a split.
	Exits []*RouteNode
	// CrossLinks lists nodes reachable by pedestrian crosswalk paths.
	CrossLinks []*RouteNode
}

// Route is the complete directed graph of nodes for one operational mode.
type Route struct {
	Entry *RouteNode
	Nodes []*RouteNode
}
