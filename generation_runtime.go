package theater

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"os"
	"sync"
	"time"

	"github.com/alex-poliushkin/theater/internal/runtimevalue"
)

type generationRuntime struct {
	metadata GenerationMetadata
	mu       sync.Mutex
	values   map[generationKey]any
}

type generationKey struct {
	bindingPath    string
	scenarioCallID string
}

func newGenerationRuntime(baseTime time.Time) *generationRuntime {
	return &generationRuntime{
		metadata: GenerationMetadata{
			Seed:     newGenerationSeed(),
			BaseTime: baseTime.UTC(),
		},
		values: make(map[generationKey]any),
	}
}

func (r *generationRuntime) Metadata() *GenerationMetadata {
	metadata := r.metadata
	return &metadata
}

func (r *generationRuntime) Resolve(
	ctx context.Context,
	resolver GeneratorResolver,
	binding bindingPlan,
	identity executionIdentity,
	refs referenceResolver,
) (any, error) {
	key := generationKey{
		bindingPath:    binding.Path,
		scenarioCallID: identity.scenarioCallID,
	}

	r.mu.Lock()
	if value, ok := r.values[key]; ok {
		r.mu.Unlock()
		return runtimevalue.Clone(value), nil
	}
	r.mu.Unlock()

	def, err := resolver.ResolveGenerator(binding.Generator)
	if err != nil {
		return nil, err
	}

	args, err := refs.ResolveBindingsContext(ctx, binding.Args)
	if err != nil {
		return nil, err
	}

	if err := validateResolvedGeneratorArgs(def.Contract, Args(args)); err != nil {
		return nil, err
	}

	if def.Validate != nil {
		if err := def.Validate(cloneValues(args)); err != nil {
			return nil, err
		}
	}

	value, err := def.Generate(GeneratorRequest{
		Args:           Args(args),
		Generation:     r.metadata,
		BindingPath:    binding.Path,
		ScenarioCallID: identity.scenarioCallID,
		ScenarioSeq:    identity.scenarioSeq,
	})
	if err != nil {
		return nil, err
	}

	if err := validateResolvedContract("generator output", def.Contract.Produces, value); err != nil {
		return nil, err
	}

	cloned := runtimevalue.Clone(value)

	r.mu.Lock()
	if cached, ok := r.values[key]; ok {
		r.mu.Unlock()
		return runtimevalue.Clone(cached), nil
	}
	r.values[key] = cloned
	r.mu.Unlock()

	return runtimevalue.Clone(cloned), nil
}

func newGenerationSeed() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		binary.BigEndian.PutUint64(raw[:8], uint64(time.Now().UTC().UnixNano()))
		binary.BigEndian.PutUint64(raw[8:], uint64(os.Getpid()))
	}

	return hex.EncodeToString(raw[:])
}
