package theater

import (
	"fmt"
	"reflect"
	"testing"
)

func TestTopologicalReachableActsUsesLexicographicReadyOrder(t *testing.T) {
	t.Parallel()

	actOrder := []string{"start", "act-2", "act-10", "finish"}
	reachableActs := reachableSet("start", "act-2", "act-10", "finish")
	successors := scopeValidationSuccessors(map[string][]string{
		"start":  {"act-2", "act-10"},
		"act-2":  {"finish"},
		"act-10": {"finish"},
	})

	got := topologicalReachableActs(actOrder, reachableActs, successors)
	want := []string{"start", "act-10", "act-2", "finish"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reachable order mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestTopologicalReachableActsMergesNewlyReadyNodesDeterministically(t *testing.T) {
	t.Parallel()

	actOrder := []string{"start", "delta", "bravo", "charlie"}
	reachableActs := reachableSet("start", "delta", "bravo", "charlie")
	successors := scopeValidationSuccessors(map[string][]string{
		"start": {"delta", "bravo"},
		"bravo": {"charlie"},
	})

	got := topologicalReachableActs(actOrder, reachableActs, successors)
	want := []string{"start", "bravo", "charlie", "delta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reachable order mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestTopologicalReachableActsSkipsUnreachableActs(t *testing.T) {
	t.Parallel()

	actOrder := []string{"start", "reachable", "orphan"}
	reachableActs := reachableSet("start", "reachable")
	successors := scopeValidationSuccessors(map[string][]string{
		"start":  {"reachable"},
		"orphan": {"start"},
	})

	got := topologicalReachableActs(actOrder, reachableActs, successors)
	want := []string{"start", "reachable"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reachable order mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestTopologicalReachableActsFallsBackToActOrderOnCycle(t *testing.T) {
	t.Parallel()

	actOrder := []string{"start", "loop-b", "loop-a", "orphan"}
	reachableActs := reachableSet("start", "loop-b", "loop-a")
	successors := scopeValidationSuccessors(map[string][]string{
		"start":  {"loop-b"},
		"loop-b": {"loop-a"},
		"loop-a": {"loop-b"},
	})

	got := topologicalReachableActs(actOrder, reachableActs, successors)
	want := []string{"start", "loop-b", "loop-a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reachable order mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func TestOrderedActQueuePopsLexicographicallyByReachableRank(t *testing.T) {
	t.Parallel()

	reachableActs := reachableSet("act-2", "act-10", "finish", "start")
	queue := newOrderedActQueue(reachableActs, len(reachableActs))

	queue.push("finish")
	queue.push("act-2")
	queue.push("start")
	queue.push("act-10")

	got := []string{
		queue.pop(),
		queue.pop(),
		queue.pop(),
		queue.pop(),
	}
	want := []string{"act-10", "act-2", "finish", "start"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("queue pop order mismatch:\n got=%v\nwant=%v", got, want)
	}
}

func BenchmarkTopologicalReachableActsLayered(b *testing.B) {
	actOrder, reachableActs, successors := layeredReachableGraph(8, 64)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		order := topologicalReachableActs(actOrder, reachableActs, successors)
		if len(order) != len(reachableActs) {
			b.Fatalf("reachable order length mismatch: got %d want %d", len(order), len(reachableActs))
		}
	}
}

func reachableSet(ids ...string) map[string]struct{} {
	reachable := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		reachable[id] = struct{}{}
	}

	return reachable
}

func scopeValidationSuccessors(edges map[string][]string) map[string][]scenarioScopeEdge {
	successors := make(map[string][]scenarioScopeEdge, len(edges))
	for fromID, toIDs := range edges {
		successors[fromID] = make([]scenarioScopeEdge, 0, len(toIDs))
		for _, toID := range toIDs {
			successors[fromID] = append(successors[fromID], scenarioScopeEdge{
				fromID: fromID,
				toID:   toID,
			})
		}
	}

	return successors
}

func layeredReachableGraph(layers, width int) ([]string, map[string]struct{}, map[string][]scenarioScopeEdge) {
	if layers < 1 {
		layers = 1
	}
	if width < 1 {
		width = 1
	}

	entryID := "layer-00-node-000"
	actOrder := []string{entryID}
	reachableActs := reachableSet(entryID)
	successors := make(map[string][]scenarioScopeEdge)
	previousLayer := []string{entryID}

	for layer := 1; layer < layers; layer++ {
		currentLayer := make([]string, 0, width)
		for node := 0; node < width; node++ {
			actID := fmt.Sprintf("layer-%02d-node-%03d", layer, node)
			actOrder = append(actOrder, actID)
			reachableActs[actID] = struct{}{}
			currentLayer = append(currentLayer, actID)
		}

		for _, fromID := range previousLayer {
			edges := make([]scenarioScopeEdge, 0, len(currentLayer))
			for _, toID := range currentLayer {
				edges = append(edges, scenarioScopeEdge{
					fromID: fromID,
					toID:   toID,
				})
			}
			successors[fromID] = edges
		}

		previousLayer = currentLayer
	}

	return actOrder, reachableActs, successors
}
