package yaml

import (
	"errors"
	"fmt"
	"io"
	"strings"

	goyaml "gopkg.in/yaml.v3"

	"github.com/alex-poliushkin/theater"
)

type rawStageSpec struct {
	ID            string                `yaml:"id"`
	Name          string                `yaml:"name,omitempty"`
	HTTP          *theater.HTTPSpec     `yaml:"http,omitempty"`
	State         *theater.StateSpec    `yaml:"state,omitempty"`
	Scenarios     []rawScenarioSpec     `yaml:"scenarios"`
	ScenarioCalls []rawScenarioCallSpec `yaml:"scenario_calls"`
	Span          theater.SourceRef
}

type rawScenarioSpec struct {
	ID     string                           `yaml:"id"`
	Name   string                           `yaml:"name,omitempty"`
	Inputs map[string]theater.ValueContract `yaml:"inputs,omitempty"`
	Acts   []rawActSpec                     `yaml:"acts"`
	Span   theater.SourceRef
}

type rawScenarioCallSpec struct {
	ID           string                           `yaml:"id"`
	Name         string                           `yaml:"name,omitempty"`
	ScenarioID   string                           `yaml:"scenario_id"`
	Bindings     map[string]rawBindingNode        `yaml:"bindings,omitempty"`
	Exports      []rawExportSpec                  `yaml:"exports,omitempty"`
	Dependencies []theater.ScenarioDependencySpec `yaml:"dependencies,omitempty"`
	Span         theater.SourceRef
}

type rawActSpec struct {
	ID           string                       `yaml:"id"`
	Name         string                       `yaml:"name,omitempty"`
	Eventually   *theater.EventuallySpec      `yaml:"eventually,omitempty"`
	Properties   map[string]rawPropertySpec   `yaml:"properties,omitempty"`
	Action       rawActionSpec                `yaml:"action"`
	CaptureAuth  *theater.HTTPAuthCaptureSpec `yaml:"capture_auth,omitempty"`
	Logs         []rawLogSpec                 `yaml:"logs,omitempty"`
	Expectations []rawExpectationSpec         `yaml:"expectations,omitempty"`
	Exports      []rawExportSpec              `yaml:"exports,omitempty"`
	Transitions  []theater.TransitionSpec     `yaml:"transitions,omitempty"`
	Span         theater.SourceRef
}

type rawActionSpec struct {
	Use        string                    `yaml:"use"`
	With       map[string]rawBindingNode `yaml:"with,omitempty"`
	Repeatable bool                      `yaml:"repeatable,omitempty"`
	Span       theater.SourceRef
}

type rawPropertySpec struct {
	Value      rawBindingNode          `yaml:"value,omitempty"`
	Inventory  *rawInventoryCall       `yaml:"inventory,omitempty"`
	Decorators []theater.DecoratorSpec `yaml:"decorators,omitempty"`
}

type rawExpectationSpec struct {
	ID      string  `yaml:"id"`
	Subject rawNode `yaml:"subject"`
	Assert  rawNode `yaml:"assert"`
	Span    theater.SourceRef
}

type rawLogSpec struct {
	ID          string                     `yaml:"id"`
	Value       rawLogValueNode            `yaml:"value,omitempty"`
	Message     string                     `yaml:"message,omitempty"`
	Fields      map[string]rawLogValueNode `yaml:"fields,omitempty"`
	Format      theater.LogFormat          `yaml:"format,omitempty"`
	Capture     theater.Capture            `yaml:"capture,omitempty"`
	Sensitivity theater.Sensitivity        `yaml:"sensitivity,omitempty"`
	Required    bool                       `yaml:"required,omitempty"`
	Span        theater.SourceRef
}

type rawLogValueNode struct {
	Node *goyaml.Node
}

type rawNode struct {
	Node *goyaml.Node
}

type rawInventoryCall struct {
	Use  string                    `yaml:"use"`
	With map[string]rawBindingNode `yaml:"with,omitempty"`
}

type rawBindingNode struct {
	Node *goyaml.Node
}

type rawBindingSpec struct {
	Kind       theater.BindingKind       `yaml:"kind"`
	Value      any                       `yaml:"value,omitempty"`
	Ref        *rawRefSpec               `yaml:"ref,omitempty"`
	Object     map[string]rawBindingNode `yaml:"object,omitempty"`
	List       []rawBindingNode          `yaml:"list,omitempty"`
	Parts      []rawBindingNode          `yaml:"parts,omitempty"`
	Generator  string                    `yaml:"generator,omitempty"`
	Env        string                    `yaml:"name,omitempty"`
	Candidates []rawBindingNode          `yaml:"candidates,omitempty"`
}

type rawExportSpec struct {
	As      string               `yaml:"as,omitempty"`
	Ref     *rawRefSpec          `yaml:"ref,omitempty"`
	Field   string               `yaml:"field,omitempty"`
	Decode  theater.DecodeKind   `yaml:"decode,omitempty"`
	Path    string               `yaml:"path,omitempty"`
	Through []rawThroughStepSpec `yaml:"through,omitempty"`
}

type rawRefSpec struct {
	Name    string               `yaml:"name"`
	Decode  theater.DecodeKind   `yaml:"decode,omitempty"`
	Path    string               `yaml:"path,omitempty"`
	Through []rawThroughStepSpec `yaml:"through,omitempty"`
}

type rawThroughStepSpec struct {
	Path      string                 `yaml:"path,omitempty"`
	Pick      *rawPickStepSpec       `yaml:"pick,omitempty"`
	Regexp    *rawRegexpStepSpec     `yaml:"regexp,omitempty"`
	Transform *theater.DecoratorSpec `yaml:"transform,omitempty"`
}

type rawPickStepSpec struct {
	At     string                   `yaml:"at,omitempty"`
	Equals rawBindingNode           `yaml:"equals,omitempty"`
	Where  []rawPickWhereClauseSpec `yaml:"where,omitempty"`
}

type rawPickWhereClauseSpec struct {
	Subject rawNode `yaml:"subject"`
	Assert  rawNode `yaml:"assert"`
}

type rawRegexpStepSpec struct {
	Pattern string `yaml:"pattern,omitempty"`
	Group   int    `yaml:"group,omitempty"`
}

func decodeRawStage(reader io.Reader) (rawStageSpec, error) {
	decoder := goyaml.NewDecoder(reader)
	decoder.KnownFields(true)

	var raw rawStageSpec
	if err := decoder.Decode(&raw); err != nil {
		return rawStageSpec{}, err
	}

	var extra any
	err := decoder.Decode(&extra)
	if err == nil {
		return rawStageSpec{}, errors.New("multiple YAML documents are not supported")
	}

	if !errors.Is(err, io.EOF) {
		return rawStageSpec{}, err
	}

	return raw, nil
}

func (r *rawStageSpec) UnmarshalYAML(node *goyaml.Node) error {
	type rawStageSpecAlias rawStageSpec

	if err := rejectUnknownFields(node, "id", "name", "http", "state", "scenarios", "scenario_calls"); err != nil {
		return err
	}

	var decoded rawStageSpecAlias
	if err := node.Decode(&decoded); err != nil {
		return err
	}

	*r = rawStageSpec(decoded)
	r.Span = rawSourceRef(node)
	return nil
}

func (r *rawScenarioSpec) UnmarshalYAML(node *goyaml.Node) error {
	type rawScenarioSpecAlias rawScenarioSpec

	if err := rejectUnknownFields(node, "id", "name", "inputs", "acts"); err != nil {
		return err
	}

	var decoded rawScenarioSpecAlias
	if err := node.Decode(&decoded); err != nil {
		return err
	}

	*r = rawScenarioSpec(decoded)
	r.Span = rawSourceRef(node)
	return nil
}

func (r *rawScenarioCallSpec) UnmarshalYAML(node *goyaml.Node) error {
	type rawScenarioCallSpecAlias rawScenarioCallSpec

	if err := rejectUnknownFields(node, "id", "name", "scenario_id", "bindings", "exports", "dependencies"); err != nil {
		return err
	}

	var decoded rawScenarioCallSpecAlias
	if err := node.Decode(&decoded); err != nil {
		return err
	}

	*r = rawScenarioCallSpec(decoded)
	r.Span = rawSourceRef(node)
	return nil
}

func (r *rawActSpec) UnmarshalYAML(node *goyaml.Node) error {
	type rawActSpecAlias rawActSpec

	if err := rejectUnknownFields(
		node,
		"id",
		"name",
		"eventually",
		"properties",
		"action",
		"capture_auth",
		"logs",
		"expectations",
		"exports",
		"transitions",
	); err != nil {
		return err
	}

	var decoded rawActSpecAlias
	if err := node.Decode(&decoded); err != nil {
		return err
	}

	*r = rawActSpec(decoded)
	r.Span = rawSourceRef(node)
	return nil
}

func (r *rawActionSpec) UnmarshalYAML(node *goyaml.Node) error {
	type rawActionSpecAlias rawActionSpec

	if err := rejectUnknownFields(node, "use", "with", "repeatable"); err != nil {
		return err
	}

	var decoded rawActionSpecAlias
	if err := node.Decode(&decoded); err != nil {
		return err
	}

	*r = rawActionSpec(decoded)
	r.Span = rawSourceRef(node)
	return nil
}

func (r *rawExpectationSpec) UnmarshalYAML(node *goyaml.Node) error {
	type rawExpectationSpecAlias rawExpectationSpec

	if err := rejectUnknownFields(node, "id", "subject", "assert"); err != nil {
		return err
	}

	var decoded rawExpectationSpecAlias
	if err := node.Decode(&decoded); err != nil {
		return err
	}

	*r = rawExpectationSpec(decoded)
	r.Span = rawSourceRef(node)
	return nil
}

func (r *rawLogSpec) UnmarshalYAML(node *goyaml.Node) error {
	type rawLogSpecAlias rawLogSpec

	if err := rejectUnknownFields(node, "id", "value", "message", "fields", "format", "capture", "sensitivity", "required"); err != nil {
		return err
	}

	var decoded rawLogSpecAlias
	if err := node.Decode(&decoded); err != nil {
		return err
	}

	*r = rawLogSpec(decoded)
	r.Span = rawSourceRef(node)
	return nil
}

func (n *rawLogValueNode) UnmarshalYAML(node *goyaml.Node) error {
	if node == nil {
		n.Node = nil
		return nil
	}

	cloned := *node
	n.Node = &cloned
	return nil
}

func (n *rawNode) UnmarshalYAML(node *goyaml.Node) error {
	if node == nil {
		n.Node = nil
		return nil
	}

	cloned := *node
	n.Node = &cloned
	return nil
}

func (n *rawBindingNode) UnmarshalYAML(node *goyaml.Node) error {
	if node == nil {
		n.Node = nil
		return nil
	}

	cloned := *node
	n.Node = &cloned
	return nil
}

func (r *rawRefSpec) UnmarshalYAML(node *goyaml.Node) error {
	switch node.Kind {
	case goyaml.ScalarNode:
		var name string
		if err := node.Decode(&name); err != nil {
			return err
		}

		*r = rawRefSpec{Name: name}
		return nil
	case goyaml.MappingNode:
		type rawRefSpecAlias rawRefSpec

		if err := rejectUnknownFields(node, "name", "decode", "path", "through"); err != nil {
			return err
		}

		var decoded rawRefSpecAlias
		if err := node.Decode(&decoded); err != nil {
			return err
		}

		*r = rawRefSpec(decoded)
		return nil
	default:
		return errors.New("ref must be string or object")
	}
}

func rawSourceRef(node *goyaml.Node) theater.SourceRef {
	if node == nil {
		return theater.SourceRef{}
	}

	return theater.SourceRef{
		Line:   node.Line,
		Column: node.Column,
	}
}

func rejectUnknownFields(node *goyaml.Node, allowed ...string) error {
	if node == nil || node.Kind != goyaml.MappingNode {
		return nil
	}

	known := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		known[key] = struct{}{}
	}

	pairs, err := mappingPairs(node)
	if err != nil {
		return err
	}

	for _, pair := range pairs {
		key := pair.key
		if _, ok := known[key.Value]; ok {
			continue
		}

		return nodeError(key, fmt.Sprintf("field %s not found in type", key.Value))
	}

	return nil
}

func nodeError(node *goyaml.Node, message string) error {
	if node == nil {
		return errors.New(message)
	}

	return fmt.Errorf("line %d, col %d: %s", node.Line, node.Column, strings.TrimSpace(message))
}
