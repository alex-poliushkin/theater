package yaml

import (
	"bytes"
	"io"
	"os"
	"reflect"

	"github.com/alex-poliushkin/theater"
)

func Decode(reader io.Reader, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	return decodeWithSource(reader, "", matchers)
}

func LoadFile(path string, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return theater.StageSpec{}, err
	}

	return decodeWithSource(bytes.NewReader(data), path, matchers)
}

func Parse(data []byte, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	return decodeWithSource(bytes.NewReader(data), "", matchers)
}

func decodeWithSource(reader io.Reader, sourceFile string, matchers theater.MatcherSugarResolver) (theater.StageSpec, error) {
	raw, err := decodeRawStage(reader)
	if err != nil {
		return theater.StageSpec{}, err
	}

	return lowerStage(raw, matchers, sourceFile)
}

func dependencyMissing(value any) bool {
	if value == nil {
		return true
	}

	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
