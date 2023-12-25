package meta

import "strings"

type Config struct {
	Source  string `json:"source" yaml:"source"`
	Configs []Rule `json:"configs" yaml:"configs"`
}

type Rule struct {
	Namespace string `json:"namespace" yaml:"namespace"`
	Path      string `json:"path" yaml:"path"`
	Dynamic   bool   `json:"dynamic" yaml:"dynamic"`
}

func (c *Config) SourceSchema() string {
	i := strings.Index(c.Source, "://")
	if i >= 0 {
		return c.Source[:i]
	}
	return ""
}

func (c *Config) SourceAddr() string {
	return c.Source[strings.Index(c.Source, "://")+3:]
}

func (r *Rule) Namespaces() []string {
	ns := strings.Split(r.Namespace, "|")
	for i := range ns {
		ns[i] = strings.TrimSpace(ns[i])
	}
	return ns
}
