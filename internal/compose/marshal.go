package compose

import "gopkg.in/yaml.v3"

type PortOverride []PortMapping

func (p PortOverride) MarshalYAML() (any, error) {
	var node yaml.Node
	if err := node.Encode([]PortMapping(p)); err != nil {
		return nil, err
	}
	node.Tag = "!override"
	return &node, nil
}
