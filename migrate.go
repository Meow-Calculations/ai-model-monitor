package main

import "gopkg.in/yaml.v3"

// parseYAML is a helper for YAML-to-SQLite migration only
func parseYAML(data []byte, out interface{}) error {
	return yaml.Unmarshal(data, out)
}