package yaml

import (
	"io"

	"github.com/alex-poliushkin/theater"
	authoringyaml "github.com/alex-poliushkin/theater/internal/authoring/yaml"
)

// Decode reads YAML from reader and lowers it into a StageSpec.
func Decode(reader io.Reader, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	return authoringyaml.Decode(reader, matchers)
}

// LoadFile loads one YAML file into a StageSpec.
func LoadFile(path string, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	return authoringyaml.LoadFile(path, matchers)
}

// LoadFlowFile loads a repo-aware flow file and linked library packages into a
// single StageSpec.
func LoadFlowFile(path string, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	return authoringyaml.LoadFlowFile(path, matchers)
}

// Parse lowers YAML bytes into a StageSpec.
func Parse(data []byte, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	return authoringyaml.Parse(data, matchers)
}
