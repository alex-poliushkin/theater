package theater

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	randv2 "math/rand/v2"
)

// GeneratorContract describes the declared args and produced value of a generator.
type GeneratorContract struct {
	Summary  string
	Args     []ArgSpec
	Produces ValueContract
}

// GeneratorRequest is the runtime request passed to a generator.
type GeneratorRequest struct {
	Args           Args
	Generation     GenerationMetadata
	BindingPath    string
	ScenarioCallID string
	ScenarioSeq    int
}

// GeneratorDef registers a generator contract together with its runtime logic.
type GeneratorDef struct {
	Contract GeneratorContract
	Validate func(args Values) error
	Generate func(request GeneratorRequest) (any, error)
}

// GeneratorRegistrar registers generators by stable name.
type GeneratorRegistrar interface {
	RegisterGenerator(ref string, generator GeneratorDef) error
}

// GeneratorResolver resolves generators by registered name.
type GeneratorResolver interface {
	ResolveGenerator(ref string) (GeneratorDef, error)
}

// DeriveRand returns a deterministic pseudo-random stream for this request and purpose.
func (r GeneratorRequest) DeriveRand(purpose string) *randv2.Rand {
	seed1, seed2 := generatorSeedPair(r.Generation.Seed, r.BindingPath, r.ScenarioCallID, purpose)
	return randv2.New(randv2.NewPCG(seed1, seed2))
}

// RunToken returns a short deterministic token derived from the run seed.
func (r GeneratorRequest) RunToken(length int) string {
	if length <= 0 {
		return ""
	}

	sum := sha256.Sum256([]byte(r.Generation.Seed))
	token := hex.EncodeToString(sum[:])
	if length >= len(token) {
		return token
	}

	return token[:length]
}

// SequenceIndex returns the stable per-scenario ordinal used by sequence-like generators.
func (r GeneratorRequest) SequenceIndex() int64 {
	if r.ScenarioSeq <= 0 {
		return 0
	}

	return int64(r.ScenarioSeq - 1)
}

func generatorSeedPair(seed, bindingPath, scenarioCallID, purpose string) (firstSeed, secondSeed uint64) {
	first := sha256.Sum256([]byte(seed + "\x00" + bindingPath + "\x00" + scenarioCallID + "\x00" + purpose))
	second := sha256.Sum256([]byte(seed + "\x01" + purpose + "\x00" + scenarioCallID + "\x00" + bindingPath))
	firstSeed = binary.BigEndian.Uint64(first[:8])
	secondSeed = binary.BigEndian.Uint64(second[:8])
	return firstSeed, secondSeed
}
