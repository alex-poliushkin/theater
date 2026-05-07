package yaml

import goyaml "gopkg.in/yaml.v3"

const errMappingKeyMissingValue = "mapping key is missing a value"

type mappingPair struct {
	key   *goyaml.Node
	value *goyaml.Node
}

func mappingPairs(node *goyaml.Node) ([]mappingPair, error) {
	if node == nil || node.Kind != goyaml.MappingNode {
		return nil, nil
	}

	if len(node.Content)%2 != 0 {
		return nil, nodeError(danglingMappingKeyNode(node), errMappingKeyMissingValue)
	}

	pairs := make([]mappingPair, 0, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		pairs = append(pairs, mappingPair{
			key:   node.Content[i],
			value: node.Content[i+1],
		})
	}

	return pairs, nil
}

func danglingMappingKeyNode(node *goyaml.Node) *goyaml.Node {
	if node == nil || len(node.Content) == 0 {
		return node
	}

	key := node.Content[len(node.Content)-1]
	if key == nil {
		return node
	}

	return key
}
