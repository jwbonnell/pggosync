package config

type Config struct {
	Exclude []string                     `yaml:"exclude"`
	Groups  map[string]map[string]string `yaml:"groups"`
}
