package theater

import (
	"sync"

	"github.com/alex-poliushkin/theater/observe"
	statemodel "github.com/alex-poliushkin/theater/state"
)

// ResourceKey identifies one scenario-scoped runtime resource.
type ResourceKey struct {
	Namespace string
	Name      string
}

// ResourceScope stores scenario-scoped reusable services keyed by ResourceKey.
type ResourceScope interface {
	GetOrCreate(key ResourceKey, create func() any) any
}

// ScenarioScopeInitializer populates and tears down scenario-local runtime
// resources.
type ScenarioScopeInitializer interface {
	InitializeScenarioScope(resources ResourceScope)
	Close()
}

// ScenarioScopeInitializerFactory builds one initializer for a run.
type ScenarioScopeInitializerFactory func() ScenarioScopeInitializer

// ScenarioScopeInitializerRegistrar registers scenario-scope initializer
// factories.
type ScenarioScopeInitializerRegistrar interface {
	RegisterScenarioScopeInitializer(ref string, factory ScenarioScopeInitializerFactory) error
}

// EventRecorder receives emitted runtime events alongside final report
// projection.
type EventRecorder interface {
	Record(Event) error
}

// ActionRequest is the runtime request passed to Action.Run.
type ActionRequest struct {
	Args        Args
	HTTP        *HTTPSpec
	State       *statemodel.Manager
	HTTPCapture *HTTPAuthCaptureSpec
	Reporter    observe.Reporter
	Paths       PathContext
	Attempt     int
	Resources   ResourceScope
}

// RunOptions configures live observation and optional raw event recording.
type RunOptions struct {
	Live            observe.Sink
	Events          EventRecorder
	ReportExporters []ReportExportSpec
	Debug           *DebugOptions
}

// NewResourceKey constructs a typed resource key from namespace and name.
func NewResourceKey(namespace, name string) ResourceKey {
	return ResourceKey{
		Namespace: namespace,
		Name:      name,
	}
}

// NewResourceScope creates an empty scenario-local resource scope.
func NewResourceScope() ResourceScope {
	return &resourceScope{
		values:   make(map[ResourceKey]any),
		inflight: make(map[ResourceKey]*resourceCell),
	}
}

type resourceScope struct {
	mu       sync.Mutex
	values   map[ResourceKey]any
	inflight map[ResourceKey]*resourceCell
}

type resourceCell struct {
	ready chan struct{}
	value any
	panic any
}

type scenarioScopeRunFactory interface {
	newScenarioScopeRun() *scenarioScopeRun
}

type scenarioScopeRun struct {
	initializers []ScenarioScopeInitializer
}

func (s *resourceScope) GetOrCreate(key ResourceKey, create func() any) any {
	if s == nil {
		return create()
	}

	s.mu.Lock()
	if value, ok := s.values[key]; ok {
		s.mu.Unlock()
		return value
	}

	if cell, ok := s.inflight[key]; ok {
		s.mu.Unlock()
		return cell.await()
	}

	cell := &resourceCell{ready: make(chan struct{})}
	s.inflight[key] = cell
	s.mu.Unlock()

	return s.createAndPublish(key, cell, create)
}

func newScenarioScopeRun(catalog runtimeCatalog) *scenarioScopeRun {
	source, ok := catalog.(scenarioScopeRunFactory)
	if !ok {
		return nil
	}

	return source.newScenarioScopeRun()
}

func (r *scenarioScopeRun) Initialize(resources ResourceScope) {
	if r == nil {
		return
	}

	for i := range r.initializers {
		if r.initializers[i] == nil {
			continue
		}

		r.initializers[i].InitializeScenarioScope(resources)
	}
}

func (r *scenarioScopeRun) Close() {
	if r == nil {
		return
	}

	for i := len(r.initializers) - 1; i >= 0; i-- {
		if r.initializers[i] == nil {
			continue
		}

		r.initializers[i].Close()
	}
}

func (s *resourceScope) createAndPublish(key ResourceKey, cell *resourceCell, create func() any) (value any) {
	panicValue := any(nil)
	defer func() {
		if recovered := recover(); recovered != nil {
			panicValue = recovered
		}

		s.mu.Lock()
		delete(s.inflight, key)
		cell.value = value
		cell.panic = panicValue
		if panicValue == nil {
			s.values[key] = value
		}
		close(cell.ready)
		s.mu.Unlock()

		if panicValue != nil {
			panic(panicValue)
		}
	}()

	value = create()
	return value
}

func (c *resourceCell) await() any {
	<-c.ready
	if c.panic != nil {
		panic(c.panic)
	}

	return c.value
}
