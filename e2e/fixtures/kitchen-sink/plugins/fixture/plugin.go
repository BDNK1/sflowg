package fixture

import (
	"fmt"

	"github.com/BDNK1/sflowg/core/plugin"
)

type Config struct {
	Prefix string `yaml:"prefix" default:"e2e"`
}

type FixturePlugin struct {
	Config Config
}

func (p *FixturePlugin) Hello(_ *plugin.Execution, input plugin.Input) (plugin.Output, error) {
	name, _ := input["name"].(string)
	if name == "" {
		name = "world"
	}

	return plugin.Output{
		"message": fmt.Sprintf("%s: hello %s", p.Config.Prefix, name),
	}, nil
}
