package theater

import (
	"fmt"
	"sort"
)

type scenarioScopeAnalysis struct {
	actEntryRoots map[string]map[string]struct{}
	reachableActs map[string]struct{}
}

type scenarioScopeEdge struct {
	carriesExports bool
	fromID         string
	toID           string
}

func analyzeScenarioScope(scenario scenarioPlan) scenarioScopeAnalysis {
	analysis := scenarioScopeAnalysis{
		actEntryRoots: make(map[string]map[string]struct{}),
		reachableActs: make(map[string]struct{}),
	}

	if len(scenario.Acts) == 0 || scenario.Acts[0].ID == "" {
		return analysis
	}

	actOrder, actsByID := uniqueScenarioActs(scenario.Acts)
	entryID := scenario.Acts[0].ID
	if _, ok := actsByID[entryID]; !ok {
		return analysis
	}

	successors, predecessors := buildScenarioScopeGraph(actOrder, actsByID)
	analysis.reachableActs = reachableActIDs(entryID, successors)

	order := topologicalReachableActs(actOrder, analysis.reachableActs, successors)
	if len(order) == 0 {
		return analysis
	}

	inputRoots := scenarioInputRoots(scenario.Inputs)
	for _, actID := range order {
		if actID == entryID {
			analysis.actEntryRoots[actID] = cloneRootSet(inputRoots)
			continue
		}

		incoming := reachablePredecessors(predecessors[actID], analysis.reachableActs)
		if len(incoming) == 0 {
			continue
		}

		var roots map[string]struct{}
		for _, edge := range incoming {
			carried := carriedActRoots(analysis.actEntryRoots[edge.fromID], actsByID[edge.fromID], edge.carriesExports)
			if roots == nil {
				roots = carried
				continue
			}

			roots = intersectRootSets(roots, carried)
		}

		analysis.actEntryRoots[actID] = roots
	}

	return analysis
}

func (a scenarioScopeAnalysis) actRoots(actID string) map[string]struct{} {
	return cloneRootSet(a.actEntryRoots[actID])
}

func (a scenarioScopeAnalysis) isReachable(actID string) bool {
	_, ok := a.reachableActs[actID]
	return ok
}

func finalScenarioRoots(scenario scenarioPlan, analysis scenarioScopeAnalysis) map[string]struct{} {
	terminalScopes := make([]map[string]struct{}, 0, len(scenario.Acts))
	for i := range scenario.Acts {
		act := scenario.Acts[i]
		if !analysis.isReachable(act.ID) || !isPassTerminalAct(act, analysis) {
			continue
		}

		terminalScopes = append(terminalScopes, carriedActRoots(analysis.actRoots(act.ID), act, true))
	}

	if len(terminalScopes) == 0 {
		return nil
	}

	roots := cloneRootSet(terminalScopes[0])
	for i := 1; i < len(terminalScopes); i++ {
		roots = intersectRootSets(roots, terminalScopes[i])
	}

	return roots
}

func validateScenarioScopeCollisions(scenario scenarioPlan, analysis scenarioScopeAnalysis) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)

	for i := range scenario.Acts {
		act := &scenario.Acts[i]
		if !analysis.isReachable(act.ID) {
			continue
		}

		entryRoots := analysis.actRoots(act.ID)
		diagnostics = append(diagnostics, validatePropertyRootCollisions(act, entryRoots)...)
		diagnostics = append(diagnostics, validateActExportRootCollisions(act, entryRoots)...)
	}

	return diagnostics
}

func validateScenarioLocalBindingRefs(scenario scenarioPlan, analysis scenarioScopeAnalysis) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)

	for i := range scenario.Acts {
		act := &scenario.Acts[i]
		if !analysis.isReachable(act.ID) {
			continue
		}

		availableRoots := analysis.actRoots(act.ID)
		if hasPropertyDependencyCycle(act.Properties) {
			for j := range act.Properties {
				if act.Properties[j].ID == "" {
					continue
				}

				availableRoots[act.Properties[j].ID] = struct{}{}
			}

			for key := range act.Action.With {
				diagnostics = append(
					diagnostics,
					validateLocalBindingRefResolution(bindingPath(act.Path+"/action", key), act.Action.With[key], availableRoots)...,
				)
			}

			continue
		}

		for j := range act.Properties {
			property := &act.Properties[j]
			if property.Inventory.Present {
				for key := range property.Inventory.With {
					diagnostics = append(
						diagnostics,
						validateLocalBindingRefResolution(bindingPath(property.Path+"/inventory/with", key), property.Inventory.With[key], availableRoots)...,
					)
				}
			}

			if property.ID != "" {
				availableRoots[property.ID] = struct{}{}
			}
		}

		for key := range act.Action.With {
			diagnostics = append(
				diagnostics,
				validateLocalBindingRefResolution(bindingPath(act.Path+"/action", key), act.Action.With[key], availableRoots)...,
			)
		}

		diagnostics = append(diagnostics, validateActExportRefAvailability(act, availableRoots)...)

		for j := range act.Logs {
			diagnostics = append(
				diagnostics,
				validateLogRefAvailability(act.Logs[j].Value, analysis.actRoots(act.ID))...,
			)
			for key := range act.Logs[j].Fields {
				diagnostics = append(
					diagnostics,
					validateLogRefAvailability(act.Logs[j].Fields[key], analysis.actRoots(act.ID))...,
				)
			}
		}
	}

	return diagnostics
}

func validateActExportRefAvailability(act *actPlan, allowedRoots map[string]struct{}) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	for i := range act.Exports {
		export := act.Exports[i]
		if export.Ref == nil || export.Ref.Name == "" {
			continue
		}

		path := exportPath(act.Path, exportAlias(export))
		if _, ok := allowedRoots[export.Ref.Name]; !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "unresolved_act_export_ref",
				Path:     path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("act export ref %q is not available in act scope at this point", export.Ref.Name),
			})
		}
	}
	return diagnostics
}

func validateExpectationOutputRootCollisions(
	act *actPlan,
	actEntryRoots map[string]struct{},
	actionOutputs map[string]ValueContract,
) []Diagnostic {
	if len(act.Expectations) == 0 || len(actionOutputs) == 0 {
		return nil
	}

	collidingRoots := cloneRootSet(actEntryRoots)
	for i := range act.Properties {
		if act.Properties[i].ID == "" {
			continue
		}

		collidingRoots[act.Properties[i].ID] = struct{}{}
	}

	ambiguousRefs := make(map[string]struct{})
	for name := range actionOutputs {
		if _, ok := collidingRoots[name]; !ok {
			continue
		}

		ambiguousRefs[name] = struct{}{}
	}
	if len(ambiguousRefs) == 0 {
		return nil
	}

	diagnostics := make([]Diagnostic, 0)
	for i := range act.Expectations {
		expectation := &act.Expectations[i]
		path := joinChildPath(act.Path, "expectation", expectation.ID)
		diagnostics = append(
			diagnostics,
			validateSelectorOutputCollision(path+"/subject", expectation.Subject.selectorPlan, ambiguousRefs)...,
		)
		for key := range expectation.Assert.Args {
			diagnostics = append(
				diagnostics,
				validateExpectationOutputRootCollisionBinding(bindingPath(path+"/assert", key), expectation.Assert.Args[key], ambiguousRefs)...,
			)
		}
	}

	return diagnostics
}

func validateExpectationArgRefs(
	act *actPlan,
	actEntryRoots map[string]struct{},
	actionOutputs map[string]ValueContract,
) []Diagnostic {
	availableRoots := cloneRootSet(actEntryRoots)
	for i := range act.Properties {
		if act.Properties[i].ID == "" {
			continue
		}

		availableRoots[act.Properties[i].ID] = struct{}{}
	}

	for name := range actionOutputs {
		availableRoots[name] = struct{}{}
	}

	diagnostics := make([]Diagnostic, 0)
	for i := range act.Expectations {
		expectation := &act.Expectations[i]
		path := joinChildPath(act.Path, "expectation", expectation.ID)
		diagnostics = append(
			diagnostics,
			validateSelectorBindingRefAvailability(
				path+"/subject",
				expectation.Subject.selectorPlan,
				availableRoots,
				func(name string) string {
					return fmt.Sprintf("binding ref %q is not available in scenario scope at this point", name)
				},
			)...,
		)
		for key := range expectation.Assert.Args {
			diagnostics = append(
				diagnostics,
				validateLocalBindingRefResolution(bindingPath(path+"/assert", key), expectation.Assert.Args[key], availableRoots)...,
			)
		}
	}

	return diagnostics
}

func validatePropertyRootCollisions(act *actPlan, actEntryRoots map[string]struct{}) []Diagnostic {
	availableRoots := cloneRootSet(actEntryRoots)
	diagnostics := make([]Diagnostic, 0)

	for i := range act.Properties {
		property := &act.Properties[i]
		if property.ID == "" {
			continue
		}

		if _, ok := availableRoots[property.ID]; ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "colliding_property_name",
				Path:     property.Path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("property key %q collides with an already-available scenario-local root", property.ID),
			})
		}

		availableRoots[property.ID] = struct{}{}
	}

	return diagnostics
}

func validateActExportRootCollisions(act *actPlan, actEntryRoots map[string]struct{}) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)

	for _, export := range act.Exports {
		alias := exportAlias(export)
		if alias == "" {
			continue
		}

		if _, ok := actEntryRoots[alias]; !ok {
			continue
		}

		diagnostics = append(diagnostics, Diagnostic{
			Code:     "colliding_act_export_name",
			Path:     exportPath(act.Path, alias),
			Severity: SeverityError,
			Summary:  fmt.Sprintf("act export name %q collides with an already-available scenario-local root", alias),
		})
	}

	return diagnostics
}

func validateExpectationOutputRootCollisionBinding(
	path string,
	binding bindingPlan,
	ambiguousRefs map[string]struct{},
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)

	switch binding.Kind {
	case BindingKindRef:
		if binding.Ref == nil || binding.Ref.Name == "" {
			return diagnostics
		}

		if _, ok := ambiguousRefs[binding.Ref.Name]; !ok {
			return validateSelectorOutputCollision(path, binding.Ref.selectorPlan, ambiguousRefs)
		}

		diagnostics = append(diagnostics, Diagnostic{
			Code:     "colliding_action_output_name",
			Path:     path,
			Severity: SeverityError,
			Summary:  fmt.Sprintf("expectation binding ref %q is ambiguous between action output and scenario-local root", binding.Ref.Name),
		})
		diagnostics = append(diagnostics, validateSelectorOutputCollision(path, binding.Ref.selectorPlan, ambiguousRefs)...)
	case BindingKindObject:
		for key := range binding.Object {
			diagnostics = append(
				diagnostics,
				validateExpectationOutputRootCollisionBinding(bindingChildPath(path, key), binding.Object[key], ambiguousRefs)...,
			)
		}
	case BindingKindList:
		for i := range binding.List {
			diagnostics = append(
				diagnostics,
				validateExpectationOutputRootCollisionBinding(fmt.Sprintf("%s[%d]", path, i), binding.List[i], ambiguousRefs)...,
			)
		}
	case BindingKindString:
		for i := range binding.Parts {
			diagnostics = append(
				diagnostics,
				validateExpectationOutputRootCollisionBinding(fmt.Sprintf("%s.parts[%d]", path, i), binding.Parts[i], ambiguousRefs)...,
			)
		}
	case BindingKindGenerate:
		for key := range binding.Args {
			diagnostics = append(
				diagnostics,
				validateExpectationOutputRootCollisionBinding(bindingChildPath(path, key), binding.Args[key], ambiguousRefs)...,
			)
		}
	}

	return diagnostics
}

func validateBindingRefResolution(path string, binding bindingPlan, allowedRoots map[string]struct{}) []Diagnostic {
	return validateBindingRefAvailability(path, binding, allowedRoots, func(name string) string {
		return fmt.Sprintf("binding ref %q is not exported by direct dependencies", name)
	})
}

func validateLocalBindingRefResolution(path string, binding bindingPlan, allowedRoots map[string]struct{}) []Diagnostic {
	return validateBindingRefAvailability(path, binding, allowedRoots, func(name string) string {
		return fmt.Sprintf("binding ref %q is not available in scenario scope at this point", name)
	})
}

func validateBindingRefAvailability(
	path string,
	binding bindingPlan,
	allowedRoots map[string]struct{},
	summary func(name string) string,
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)

	switch binding.Kind {
	case BindingKindRef:
		if binding.Ref == nil || binding.Ref.Name == "" {
			return diagnostics
		}

		if _, ok := allowedRoots[binding.Ref.Name]; ok {
			return append(diagnostics, validateSelectorBindingRefAvailability(path, binding.Ref.selectorPlan, allowedRoots, summary)...)
		}

		diagnostics = append(diagnostics, Diagnostic{
			Code:     "unresolved_binding_ref",
			Path:     path,
			Severity: SeverityError,
			Summary:  summary(binding.Ref.Name),
		})
		diagnostics = append(diagnostics, validateSelectorBindingRefAvailability(path, binding.Ref.selectorPlan, allowedRoots, summary)...)
	case BindingKindObject:
		for key := range binding.Object {
			diagnostics = append(
				diagnostics,
				validateBindingRefAvailability(bindingChildPath(path, key), binding.Object[key], allowedRoots, summary)...,
			)
		}
	case BindingKindList:
		for i := range binding.List {
			diagnostics = append(
				diagnostics,
				validateBindingRefAvailability(fmt.Sprintf("%s[%d]", path, i), binding.List[i], allowedRoots, summary)...,
			)
		}
	case BindingKindString:
		for i := range binding.Parts {
			diagnostics = append(
				diagnostics,
				validateBindingRefAvailability(fmt.Sprintf("%s.parts[%d]", path, i), binding.Parts[i], allowedRoots, summary)...,
			)
		}
	case BindingKindGenerate:
		for key := range binding.Args {
			diagnostics = append(
				diagnostics,
				validateBindingRefAvailability(bindingChildPath(path, key), binding.Args[key], allowedRoots, summary)...,
			)
		}
	}

	return diagnostics
}

func validateSelectorBindingRefAvailability(
	path string,
	selector selectorPlan,
	allowedRoots map[string]struct{},
	summary func(name string) string,
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	for i := range selector.Through {
		if selector.Through[i].Pick == nil {
			continue
		}

		diagnostics = append(
			diagnostics,
			validateBindingRefAvailability(
				fmt.Sprintf("%s/through[%d]/pick/equals", path, i),
				selector.Through[i].Pick.Equals,
				allowedRoots,
				summary,
			)...,
		)
	}

	return diagnostics
}

func validateLogRefAvailability(value logValuePlan, allowedRoots map[string]struct{}) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	if value.Ref != "" {
		if _, ok := allowedRoots[value.Ref]; !ok {
			diagnostics = append(diagnostics, Diagnostic{
				Code:     "unresolved_log_ref",
				Path:     value.Path,
				Severity: SeverityError,
				Summary:  fmt.Sprintf("log ref %q is not available in scenario scope at this point", value.Ref),
			})
		}
	}

	diagnostics = append(
		diagnostics,
		validateSelectorBindingRefAvailability(
			value.Path,
			value.selectorPlan,
			allowedRoots,
			func(name string) string {
				return fmt.Sprintf("binding ref %q is not available in scenario scope at this point", name)
			},
		)...,
	)

	for key := range value.Object {
		diagnostics = append(diagnostics, validateLogRefAvailability(value.Object[key], allowedRoots)...)
	}
	for i := range value.List {
		diagnostics = append(diagnostics, validateLogRefAvailability(value.List[i], allowedRoots)...)
	}

	return diagnostics
}

func validateSelectorOutputCollision(
	path string,
	selector selectorPlan,
	ambiguousRefs map[string]struct{},
) []Diagnostic {
	diagnostics := make([]Diagnostic, 0)
	for i := range selector.Through {
		if selector.Through[i].Pick == nil {
			continue
		}

		diagnostics = append(
			diagnostics,
			validateExpectationOutputRootCollisionBinding(
				fmt.Sprintf("%s/through[%d]/pick/equals", path, i),
				selector.Through[i].Pick.Equals,
				ambiguousRefs,
			)...,
		)
	}

	return diagnostics
}

func buildScenarioScopeGraph(
	actOrder []string,
	actsByID map[string]actPlan,
) (
	successors map[string][]scenarioScopeEdge,
	predecessors map[string][]scenarioScopeEdge,
) {
	successors = make(map[string][]scenarioScopeEdge, len(actOrder))
	predecessors = make(map[string][]scenarioScopeEdge, len(actOrder))

	for _, actID := range actOrder {
		act := actsByID[actID]
		for _, transition := range act.Transitions {
			if transition.To == "" {
				continue
			}

			if _, ok := actsByID[transition.To]; !ok {
				continue
			}

			edge := scenarioScopeEdge{
				carriesExports: transition.On == TransitionOnPass,
				fromID:         actID,
				toID:           transition.To,
			}
			successors[actID] = append(successors[actID], edge)
			predecessors[transition.To] = append(predecessors[transition.To], edge)
		}
	}

	for _, actID := range actOrder {
		sortScenarioScopeEdges(successors[actID])
		sortScenarioScopeEdges(predecessors[actID])
	}

	return successors, predecessors
}

func carriedActRoots(roots map[string]struct{}, act actPlan, carriesExports bool) map[string]struct{} {
	carried := cloneRootSet(roots)
	if !carriesExports {
		return carried
	}

	for _, export := range act.Exports {
		alias := exportAlias(export)
		if alias == "" {
			continue
		}

		carried[alias] = struct{}{}
	}

	return carried
}

func isPassTerminalAct(act actPlan, analysis scenarioScopeAnalysis) bool {
	for _, transition := range act.Transitions {
		if transition.On != TransitionOnPass {
			continue
		}

		if transition.To == "" || !analysis.isReachable(transition.To) {
			continue
		}

		return false
	}

	return true
}

func cloneRootSet(roots map[string]struct{}) map[string]struct{} {
	cloned := make(map[string]struct{}, len(roots))
	for root := range roots {
		cloned[root] = struct{}{}
	}

	return cloned
}

func intersectRootSets(left, right map[string]struct{}) map[string]struct{} {
	intersection := make(map[string]struct{})
	for root := range left {
		if _, ok := right[root]; ok {
			intersection[root] = struct{}{}
		}
	}

	return intersection
}

func reachableActIDs(entryID string, successors map[string][]scenarioScopeEdge) map[string]struct{} {
	reachable := make(map[string]struct{})
	queue := []string{entryID}

	for len(queue) != 0 {
		current := queue[0]
		queue = queue[1:]
		if _, ok := reachable[current]; ok {
			continue
		}

		reachable[current] = struct{}{}
		for _, edge := range successors[current] {
			if _, ok := reachable[edge.toID]; ok {
				continue
			}

			queue = append(queue, edge.toID)
		}
	}

	return reachable
}

func reachablePredecessors(
	edges []scenarioScopeEdge,
	reachableActs map[string]struct{},
) []scenarioScopeEdge {
	reachable := make([]scenarioScopeEdge, 0, len(edges))
	for _, edge := range edges {
		if _, ok := reachableActs[edge.fromID]; !ok {
			continue
		}

		reachable = append(reachable, edge)
	}

	return reachable
}

func scenarioInputRoots(inputs map[string]ValueContract) map[string]struct{} {
	roots := make(map[string]struct{}, len(inputs))
	for name := range inputs {
		roots[name] = struct{}{}
	}

	return roots
}

func sortScenarioScopeEdges(edges []scenarioScopeEdge) {
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].toID != edges[j].toID {
			return edges[i].toID < edges[j].toID
		}

		if edges[i].fromID != edges[j].fromID {
			return edges[i].fromID < edges[j].fromID
		}

		if edges[i].carriesExports == edges[j].carriesExports {
			return false
		}

		return !edges[i].carriesExports && edges[j].carriesExports
	})
}

func topologicalReachableActs(
	actOrder []string,
	reachableActs map[string]struct{},
	successors map[string][]scenarioScopeEdge,
) []string {
	indegree := make(map[string]int, len(reachableActs))
	for actID := range reachableActs {
		indegree[actID] = 0
	}

	for actID := range reachableActs {
		for _, edge := range successors[actID] {
			if _, ok := reachableActs[edge.toID]; !ok {
				continue
			}

			indegree[edge.toID]++
		}
	}

	ready := newOrderedActQueue(reachableActs, len(indegree))
	for actID, degree := range indegree {
		if degree == 0 {
			ready.push(actID)
		}
	}

	order := make([]string, 0, len(reachableActs))
	for ready.Len() != 0 {
		current := ready.pop()
		order = append(order, current)

		for _, edge := range successors[current] {
			if _, ok := reachableActs[edge.toID]; !ok {
				continue
			}

			indegree[edge.toID]--
			if indegree[edge.toID] == 0 {
				ready.push(edge.toID)
			}
		}
	}

	if len(order) == len(reachableActs) {
		return order
	}

	return orderedReachableActs(actOrder, reachableActs)
}

func orderedReachableActs(actOrder []string, reachableActs map[string]struct{}) []string {
	ordered := make([]string, 0, len(reachableActs))
	for _, actID := range actOrder {
		if _, ok := reachableActs[actID]; !ok {
			continue
		}

		ordered = append(ordered, actID)
	}

	return ordered
}

type orderedActQueue struct {
	ids  []orderedActRef
	rank map[string]int
}

type orderedActRef struct {
	id   string
	rank int
}

func newOrderedActQueue(reachableActs map[string]struct{}, capacity int) orderedActQueue {
	orderedIDs := make([]string, 0, len(reachableActs))
	for actID := range reachableActs {
		orderedIDs = append(orderedIDs, actID)
	}
	sort.Strings(orderedIDs)

	rank := make(map[string]int, len(orderedIDs))
	for i, actID := range orderedIDs {
		rank[actID] = i
	}

	return orderedActQueue{
		ids:  make([]orderedActRef, 0, capacity),
		rank: rank,
	}
}

func (q *orderedActQueue) Len() int {
	return len(q.ids)
}

func (q *orderedActQueue) less(i, j int) bool {
	return q.ids[i].rank < q.ids[j].rank
}

func (q *orderedActQueue) swap(i, j int) {
	q.ids[i], q.ids[j] = q.ids[j], q.ids[i]
}

func (q *orderedActQueue) push(actID string) {
	q.ids = append(q.ids, orderedActRef{id: actID, rank: q.rank[actID]})
	q.up(len(q.ids) - 1)
}

func (q *orderedActQueue) pop() string {
	last := len(q.ids) - 1
	q.swap(0, last)

	value := q.ids[last]
	q.ids = q.ids[:last]
	if len(q.ids) != 0 {
		q.down(0)
	}

	return value.id
}

func (q *orderedActQueue) up(index int) {
	for index > 0 {
		parent := (index - 1) / 2
		if !q.less(index, parent) {
			return
		}

		q.swap(index, parent)
		index = parent
	}
}

func (q *orderedActQueue) down(index int) {
	for {
		left := index*2 + 1
		if left >= len(q.ids) {
			return
		}

		smallest := left
		right := left + 1
		if right < len(q.ids) && q.less(right, left) {
			smallest = right
		}
		if !q.less(smallest, index) {
			return
		}

		q.swap(index, smallest)
		index = smallest
	}
}

func uniqueScenarioActs(acts []actPlan) (order []string, actsByID map[string]actPlan) {
	order = make([]string, 0, len(acts))
	actsByID = make(map[string]actPlan, len(acts))

	for i := range acts {
		act := acts[i]
		if act.ID == "" {
			continue
		}

		if _, ok := actsByID[act.ID]; ok {
			continue
		}

		order = append(order, act.ID)
		actsByID[act.ID] = act
	}

	return order, actsByID
}
