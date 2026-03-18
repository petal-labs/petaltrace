package diff

import (
	"github.com/petal-labs/petaltrace/store"
)

// StructuralDiffer compares execution paths between runs
type StructuralDiffer struct{}

// NewStructuralDiffer creates a new structural differ
func NewStructuralDiffer() *StructuralDiffer {
	return &StructuralDiffer{}
}

// CompareGraphs compares the execution paths of two runs
func (d *StructuralDiffer) CompareGraphs(baseSpans, compareSpans []*store.Span) *store.GraphDiff {
	baseNodes := extractNodeIDs(baseSpans)
	compareNodes := extractNodeIDs(compareSpans)

	diff := &store.GraphDiff{
		NodesAdded:   make([]string, 0),
		NodesRemoved: make([]string, 0),
		EdgesChanged: make([]string, 0),
	}

	// Find nodes in compare but not in base (added)
	for nodeID := range compareNodes {
		if _, exists := baseNodes[nodeID]; !exists {
			diff.NodesAdded = append(diff.NodesAdded, nodeID)
		}
	}

	// Find nodes in base but not in compare (removed)
	for nodeID := range baseNodes {
		if _, exists := compareNodes[nodeID]; !exists {
			diff.NodesRemoved = append(diff.NodesRemoved, nodeID)
		}
	}

	// Compare edges (parent-child relationships)
	baseEdges := extractEdges(baseSpans)
	compareEdges := extractEdges(compareSpans)

	for edge := range compareEdges {
		if _, exists := baseEdges[edge]; !exists {
			diff.EdgesChanged = append(diff.EdgesChanged, edge+" (added)")
		}
	}
	for edge := range baseEdges {
		if _, exists := compareEdges[edge]; !exists {
			diff.EdgesChanged = append(diff.EdgesChanged, edge+" (removed)")
		}
	}

	return diff
}

// HasPathDivergence returns true if the execution paths differ
func (d *StructuralDiffer) HasPathDivergence(graphDiff *store.GraphDiff) bool {
	if graphDiff == nil {
		return false
	}
	return len(graphDiff.NodesAdded) > 0 ||
		len(graphDiff.NodesRemoved) > 0 ||
		len(graphDiff.EdgesChanged) > 0
}

// MatchNodes matches spans between two runs by node ID
func (d *StructuralDiffer) MatchNodes(baseSpans, compareSpans []*store.Span) []NodeMatch {
	baseByNode := groupSpansByNode(baseSpans)
	compareByNode := groupSpansByNode(compareSpans)

	var matches []NodeMatch

	// All nodes from both runs
	allNodes := make(map[string]bool)
	for nodeID := range baseByNode {
		allNodes[nodeID] = true
	}
	for nodeID := range compareByNode {
		allNodes[nodeID] = true
	}

	for nodeID := range allNodes {
		baseSpan := baseByNode[nodeID]
		compareSpan := compareByNode[nodeID]

		match := NodeMatch{
			NodeID:      nodeID,
			BaseSpan:    baseSpan,
			CompareSpan: compareSpan,
		}

		if baseSpan != nil && compareSpan != nil {
			match.Status = MatchStatusBoth
			if baseSpan.Node != nil {
				match.NodeType = baseSpan.Node.NodeType
			}
		} else if baseSpan != nil {
			match.Status = MatchStatusBaseOnly
			if baseSpan.Node != nil {
				match.NodeType = baseSpan.Node.NodeType
			}
		} else {
			match.Status = MatchStatusCompareOnly
			if compareSpan.Node != nil {
				match.NodeType = compareSpan.Node.NodeType
			}
		}

		matches = append(matches, match)
	}

	return matches
}

// NodeMatch represents a matched pair of spans from two runs
type NodeMatch struct {
	NodeID      string
	NodeType    string
	Status      MatchStatus
	BaseSpan    *store.Span
	CompareSpan *store.Span
}

// MatchStatus indicates whether a node exists in base, compare, or both runs
type MatchStatus string

const (
	MatchStatusBoth        MatchStatus = "both"
	MatchStatusBaseOnly    MatchStatus = "base_only"
	MatchStatusCompareOnly MatchStatus = "compare_only"
)

// extractNodeIDs extracts unique node IDs from spans
func extractNodeIDs(spans []*store.Span) map[string]bool {
	nodes := make(map[string]bool)
	for _, span := range spans {
		if span.Node != nil && span.Node.NodeID != "" {
			nodes[span.Node.NodeID] = true
		}
	}
	return nodes
}

// extractEdges extracts parent-child relationships as edge strings
func extractEdges(spans []*store.Span) map[string]bool {
	edges := make(map[string]bool)
	spanIDToNode := make(map[string]string)

	// Build span ID to node ID mapping
	for _, span := range spans {
		if span.Node != nil && span.Node.NodeID != "" {
			spanIDToNode[span.ID] = span.Node.NodeID
		}
	}

	// Extract edges based on parent relationships
	for _, span := range spans {
		if span.ParentID != nil && *span.ParentID != "" && span.Node != nil {
			parentNode := spanIDToNode[*span.ParentID]
			if parentNode != "" {
				edge := parentNode + " -> " + span.Node.NodeID
				edges[edge] = true
			}
		}
	}

	return edges
}

// groupSpansByNode groups spans by their node ID, keeping only the first LLM span per node
func groupSpansByNode(spans []*store.Span) map[string]*store.Span {
	result := make(map[string]*store.Span)
	for _, span := range spans {
		if span.Node != nil && span.Node.NodeID != "" {
			// Prefer LLM spans for comparison
			if existing, ok := result[span.Node.NodeID]; ok {
				if span.Kind == store.SpanKindLLM && existing.Kind != store.SpanKindLLM {
					result[span.Node.NodeID] = span
				}
			} else {
				result[span.Node.NodeID] = span
			}
		}
	}
	return result
}
