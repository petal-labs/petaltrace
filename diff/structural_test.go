package diff

import (
	"testing"

	"github.com/petal-labs/petaltrace/store"
)

func TestStructuralDiffer_CompareGraphs(t *testing.T) {
	d := NewStructuralDiffer()

	tests := []struct {
		name          string
		baseSpans     []*store.Span
		compareSpans  []*store.Span
		wantAdded     int
		wantRemoved   int
		wantChanged   int
	}{
		{
			name:          "empty spans",
			baseSpans:     []*store.Span{},
			compareSpans:  []*store.Span{},
			wantAdded:     0,
			wantRemoved:   0,
			wantChanged:   0,
		},
		{
			name: "identical graphs",
			baseSpans: []*store.Span{
				{ID: "s1", Node: &store.NodeSpanData{NodeID: "node1", NodeType: "agent"}},
				{ID: "s2", Node: &store.NodeSpanData{NodeID: "node2", NodeType: "task"}},
			},
			compareSpans: []*store.Span{
				{ID: "s1", Node: &store.NodeSpanData{NodeID: "node1", NodeType: "agent"}},
				{ID: "s2", Node: &store.NodeSpanData{NodeID: "node2", NodeType: "task"}},
			},
			wantAdded:   0,
			wantRemoved: 0,
			wantChanged: 0,
		},
		{
			name: "node added",
			baseSpans: []*store.Span{
				{ID: "s1", Node: &store.NodeSpanData{NodeID: "node1", NodeType: "agent"}},
			},
			compareSpans: []*store.Span{
				{ID: "s1", Node: &store.NodeSpanData{NodeID: "node1", NodeType: "agent"}},
				{ID: "s2", Node: &store.NodeSpanData{NodeID: "node2", NodeType: "task"}},
			},
			wantAdded:   1,
			wantRemoved: 0,
			wantChanged: 0,
		},
		{
			name: "node removed",
			baseSpans: []*store.Span{
				{ID: "s1", Node: &store.NodeSpanData{NodeID: "node1", NodeType: "agent"}},
				{ID: "s2", Node: &store.NodeSpanData{NodeID: "node2", NodeType: "task"}},
			},
			compareSpans: []*store.Span{
				{ID: "s1", Node: &store.NodeSpanData{NodeID: "node1", NodeType: "agent"}},
			},
			wantRemoved: 1,
			wantAdded:   0,
			wantChanged: 0,
		},
		{
			name: "node replaced",
			baseSpans: []*store.Span{
				{ID: "s1", Node: &store.NodeSpanData{NodeID: "node1", NodeType: "agent"}},
			},
			compareSpans: []*store.Span{
				{ID: "s2", Node: &store.NodeSpanData{NodeID: "node2", NodeType: "task"}},
			},
			wantAdded:   1,
			wantRemoved: 1,
			wantChanged: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := d.CompareGraphs(tt.baseSpans, tt.compareSpans)

			if len(result.NodesAdded) != tt.wantAdded {
				t.Errorf("NodesAdded = %d, want %d", len(result.NodesAdded), tt.wantAdded)
			}
			if len(result.NodesRemoved) != tt.wantRemoved {
				t.Errorf("NodesRemoved = %d, want %d", len(result.NodesRemoved), tt.wantRemoved)
			}
			if len(result.EdgesChanged) != tt.wantChanged {
				t.Errorf("EdgesChanged = %d, want %d", len(result.EdgesChanged), tt.wantChanged)
			}
		})
	}
}

func TestStructuralDiffer_HasPathDivergence(t *testing.T) {
	d := NewStructuralDiffer()

	tests := []struct {
		name      string
		graphDiff *store.GraphDiff
		want      bool
	}{
		{
			name:      "nil diff",
			graphDiff: nil,
			want:      false,
		},
		{
			name: "no divergence",
			graphDiff: &store.GraphDiff{
				NodesAdded:   []string{},
				NodesRemoved: []string{},
				EdgesChanged: []string{},
			},
			want: false,
		},
		{
			name: "nodes added",
			graphDiff: &store.GraphDiff{
				NodesAdded:   []string{"node1"},
				NodesRemoved: []string{},
				EdgesChanged: []string{},
			},
			want: true,
		},
		{
			name: "nodes removed",
			graphDiff: &store.GraphDiff{
				NodesAdded:   []string{},
				NodesRemoved: []string{"node1"},
				EdgesChanged: []string{},
			},
			want: true,
		},
		{
			name: "edges changed",
			graphDiff: &store.GraphDiff{
				NodesAdded:   []string{},
				NodesRemoved: []string{},
				EdgesChanged: []string{"node1 -> node2 (added)"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := d.HasPathDivergence(tt.graphDiff); got != tt.want {
				t.Errorf("HasPathDivergence() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStructuralDiffer_MatchNodes(t *testing.T) {
	d := NewStructuralDiffer()

	baseSpans := []*store.Span{
		{ID: "s1", Kind: store.SpanKindNode, Node: &store.NodeSpanData{NodeID: "node1", NodeType: "agent"}},
		{ID: "s2", Kind: store.SpanKindNode, Node: &store.NodeSpanData{NodeID: "node2", NodeType: "task"}},
	}

	compareSpans := []*store.Span{
		{ID: "s3", Kind: store.SpanKindNode, Node: &store.NodeSpanData{NodeID: "node2", NodeType: "task"}},
		{ID: "s4", Kind: store.SpanKindNode, Node: &store.NodeSpanData{NodeID: "node3", NodeType: "tool"}},
	}

	matches := d.MatchNodes(baseSpans, compareSpans)

	// Should have 3 matches: node1 (base only), node2 (both), node3 (compare only)
	if len(matches) != 3 {
		t.Fatalf("Expected 3 matches, got %d", len(matches))
	}

	// Check each match
	matchMap := make(map[string]NodeMatch)
	for _, m := range matches {
		matchMap[m.NodeID] = m
	}

	if m, ok := matchMap["node1"]; !ok || m.Status != MatchStatusBaseOnly {
		t.Errorf("node1 should be base_only, got %v", m.Status)
	}

	if m, ok := matchMap["node2"]; !ok || m.Status != MatchStatusBoth {
		t.Errorf("node2 should be both, got %v", m.Status)
	}

	if m, ok := matchMap["node3"]; !ok || m.Status != MatchStatusCompareOnly {
		t.Errorf("node3 should be compare_only, got %v", m.Status)
	}
}

func TestExtractNodeIDs(t *testing.T) {
	spans := []*store.Span{
		{ID: "s1", Node: &store.NodeSpanData{NodeID: "node1"}},
		{ID: "s2", Node: &store.NodeSpanData{NodeID: "node2"}},
		{ID: "s3", Node: nil},
		{ID: "s4", Node: &store.NodeSpanData{NodeID: ""}},
	}

	nodeIDs := extractNodeIDs(spans)

	if len(nodeIDs) != 2 {
		t.Errorf("Expected 2 node IDs, got %d", len(nodeIDs))
	}

	if !nodeIDs["node1"] {
		t.Error("node1 should be in result")
	}

	if !nodeIDs["node2"] {
		t.Error("node2 should be in result")
	}
}

func TestExtractEdges(t *testing.T) {
	parentID := "s1"
	spans := []*store.Span{
		{ID: "s1", Node: &store.NodeSpanData{NodeID: "node1"}},
		{ID: "s2", ParentID: &parentID, Node: &store.NodeSpanData{NodeID: "node2"}},
	}

	edges := extractEdges(spans)

	if len(edges) != 1 {
		t.Errorf("Expected 1 edge, got %d", len(edges))
	}

	expectedEdge := "node1 -> node2"
	if !edges[expectedEdge] {
		t.Errorf("Expected edge '%s' not found", expectedEdge)
	}
}

func TestGroupSpansByNode(t *testing.T) {
	spans := []*store.Span{
		{ID: "s1", Kind: store.SpanKindNode, Node: &store.NodeSpanData{NodeID: "node1"}},
		{ID: "s2", Kind: store.SpanKindLLM, Node: &store.NodeSpanData{NodeID: "node1"}},
		{ID: "s3", Kind: store.SpanKindNode, Node: &store.NodeSpanData{NodeID: "node2"}},
	}

	result := groupSpansByNode(spans)

	if len(result) != 2 {
		t.Errorf("Expected 2 nodes, got %d", len(result))
	}

	// LLM span should be preferred for node1
	if result["node1"].Kind != store.SpanKindLLM {
		t.Error("LLM span should be preferred for node1")
	}

	if result["node2"].Kind != store.SpanKindNode {
		t.Error("node2 should have node span")
	}
}
