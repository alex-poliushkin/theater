package theater_test

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/alex-poliushkin/theater"
	"github.com/alex-poliushkin/theater/builtin"
	builtinaction "github.com/alex-poliushkin/theater/builtin/action"
	builtinexpectation "github.com/alex-poliushkin/theater/builtin/expectation"
	"github.com/alex-poliushkin/theater/observe"
	pluginregistry "github.com/alex-poliushkin/theater/plugin/registry"
)

const (
	embeddingActionPath      = "stage.embedding/call.smoke/act.generate/action"
	embeddingExpectationPath = "stage.embedding/call.smoke/act.generate/expectation.message"
)

func ExampleRunOptions() {
	bundle, err := builtin.NewBundle()
	if err != nil {
		panic(err)
	}

	recorder := &embeddingEventRecorder{}
	live := &embeddingLiveSink{}
	result, err := theater.NewRunner(bundle.Catalog, bundle.Matchers).Run(
		context.Background(),
		embeddingStageSpec(),
		theater.RunOptions{
			Events: recorder,
			Live:   live,
		},
	)
	if err != nil {
		panic(err)
	}

	document, err := theater.NewProjector().Document(recorder.Events())
	if err != nil {
		panic(err)
	}

	events := recorder.Events()
	fmt.Println("result:", result.Report.Status)
	fmt.Println("action event:", embeddingHasEvent(events, theater.EventKindActionFinished, embeddingActionPath))
	fmt.Println("expectation event:", embeddingHasEvent(events, theater.EventKindExpectationFinished, embeddingExpectationPath))
	fmt.Println("live action:", live.HasTransition(theater.EventKindActionFinished, embeddingActionPath))
	fmt.Println("projected:", document.Report.Status)

	// Output:
	// result: passed
	// action event: true
	// expectation event: true
	// live action: true
	// projected: passed
}

func ExampleValidator_ListDebugPaths() {
	bundle, err := builtin.NewBundle()
	if err != nil {
		panic(err)
	}

	listing, err := theater.NewValidator(bundle.Catalog, bundle.Matchers).ListDebugPaths(embeddingStageSpec())
	if err != nil {
		panic(err)
	}

	fmt.Println("diagnostics:", len(listing.Diagnostics))
	for _, path := range listing.Paths {
		if path.Kind == theater.DebugBoundaryKindAction && path.Phase == theater.DebugBoundaryPhaseBefore {
			fmt.Println("action:", path.Path)
			break
		}
	}

	// Output:
	// diagnostics: 0
	// action: stage.embedding/call.smoke/act.generate/action
}

func ExampleNewPluginCatalog() {
	bundle, err := builtin.NewBundle()
	if err != nil {
		panic(err)
	}

	plugins, err := theater.NewPluginCatalog(bundle.Catalog, bundle.Matchers, theater.PluginCatalogOptions{
		RootDir: ".",
		Config: pluginregistry.ConfigFile{
			Schema: pluginregistry.ConfigSchemaVersion,
			Plugins: map[string]pluginregistry.PluginEntry{
				"smoke-plugin": {
					Manifest: filepath.Join("testdata", "plugins", "smoke", "manifest.json"),
					Exec: pluginregistry.ExecSpec{
						Command: []string{filepath.Join("testdata", "plugins", "smoke", "smoke.py")},
					},
					AllowCapabilities: []string{
						"action.smoke.echo",
						"matcher.smoke.equal",
					},
				},
			},
		},
	})
	if err != nil {
		panic(err)
	}

	_, actionErr := plugins.ResolveAction("action.smoke.echo")
	_, matcherErr := plugins.Resolve("matcher.smoke.equal")
	_, baseActionErr := plugins.ResolveAction(builtinaction.GenerateRef)
	_, baseMatcherErr := plugins.Resolve(builtinexpectation.EqualRef)
	fmt.Println(actionErr == nil)
	fmt.Println(matcherErr == nil)
	fmt.Println(baseActionErr == nil)
	fmt.Println(baseMatcherErr == nil)

	// Output:
	// true
	// true
	// true
	// true
}

type embeddingEventRecorder struct {
	mu     sync.Mutex
	events []theater.Event
}

type embeddingLiveSink struct {
	mu        sync.Mutex
	envelopes []observe.Envelope
}

func (r *embeddingEventRecorder) Record(event theater.Event) error {
	if err := event.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event)
	return nil
}

func (r *embeddingEventRecorder) Events() []theater.Event {
	r.mu.Lock()
	defer r.mu.Unlock()

	events := make([]theater.Event, len(r.events))
	copy(events, r.events)
	return events
}

func (s *embeddingLiveSink) Publish(envelope observe.Envelope) uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.envelopes = append(s.envelopes, envelope)
	return uint64(len(s.envelopes))
}

func (s *embeddingLiveSink) HasTransition(kind, path string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, envelope := range s.envelopes {
		if envelope.Transition != nil &&
			envelope.Transition.EventKind == kind &&
			envelope.Node.Path == path &&
			envelope.DurableMirror {
			return true
		}
	}

	return false
}

func embeddingHasEvent(events []theater.Event, kind, path string) bool {
	for _, event := range events {
		if event.Kind == kind && event.Path == path {
			return true
		}
	}

	return false
}

func embeddingStageSpec() theater.StageSpec {
	return theater.StageSpec{
		ID: "embedding",
		Scenarios: []theater.ScenarioSpec{
			{
				ID: "smoke",
				Acts: []theater.ActSpec{
					{
						ID: "generate",
						Action: theater.ActionSpec{
							Use: "action.generate",
							With: map[string]theater.BindingSpec{
								"outputs": embeddingLiteral(map[string]any{"message": "hello"}),
							},
						},
						Expectations: []theater.ExpectationSpec{
							{
								ID:      "message",
								Subject: theater.SubjectSpec{Field: "values"},
								Assert: theater.AssertSpec{
									Ref: builtinexpectation.EqualRef,
									Args: map[string]theater.BindingSpec{
										"expected": embeddingLiteral(map[string]any{"message": "hello"}),
									},
								},
							},
						},
					},
				},
			},
		},
		ScenarioCalls: []theater.ScenarioCallSpec{
			{ID: "smoke", ScenarioID: "smoke"},
		},
	}
}

func embeddingLiteral(value any) theater.BindingSpec {
	return theater.BindingSpec{
		Kind:  theater.BindingKindLiteral,
		Value: value,
	}
}
