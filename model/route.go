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
	// NodeQueue is the queueing area before service zones.
	NodeQueue NodeType = "Queue"
	// NodeIdentifier is a license-plate / ID check station.
	NodeIdentifier NodeType = "Identifier"
	// NodeSplit divides traffic into two or more lanes.
	NodeSplit NodeType = "Split"
	// NodeJoiner merges multiple lanes back into one.
	NodeJoiner NodeType = "Joiner"
	// NodeCrosswalk allows pedestrians to cross between paths or
	// reach a waiting area / exit.
	NodeCrosswalk NodeType = "Crosswalk"
	// NodeDropZone is where passengers exit the vehicle (drop-off mode).
	NodeDropZone NodeType = "DropZone"
	// NodeWaitZone is where a vehicle parks while passengers are notified.
	NodeWaitZone NodeType = "WaitZone"
	// NodeServiceZone is a shared pickup/dropoff location.
	NodeServiceZone NodeType = "ServiceZone"
	// NodeBuilding is the final pedestrian destination.
	NodeBuilding NodeType = "Building"
	// NodeExit is where a vehicle leaves the facility.
	NodeExit NodeType = "Exit"
)

// RouteNode is a single vertex in the route graph.
type RouteNode struct {
	ID         string
	Type       NodeType
	Label      string
	Pos        Point
	// RequiresIDCheck makes ID check attachable to any node.
	RequiresIDCheck bool
	Exits      []*RouteNode
	CrossLinks []*RouteNode
}

// Route is the complete directed graph of nodes for one operational mode.
type Route struct {
	Entry *RouteNode
	Nodes []*RouteNode
}

// FindNode returns the node with the given ID, or nil.
func (r *Route) FindNode(id string) *RouteNode {
	for _, n := range r.Nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}
