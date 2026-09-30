package router

// LogicalEdge defines a directed edge in the rule graph with an explicit emit label
type LogicalEdge struct {
	Label string `json:"label"`
	To    string `json:"to"`
}

// RouterNode represents a node in the router's view of the graph
type RouterNode struct {
	NodeID       string
	LogicalEdges map[string]string // label -> toNodeID
}
