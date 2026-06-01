package compose

import "gopkg.in/yaml.v3"

type PortOverride []PortMapping

type ResetSequence struct{}

func (r ResetSequence) MarshalYAML() (any, error) {
	var node yaml.Node
	node.Tag = "!reset"
	node.Kind = yaml.SequenceNode
	return &node, nil
}
