package htxt

import (
	"os"
	"strings"

	"github.com/herdworks/httt"
)

type Environment struct {
	Variables    map[string]string
	TemplateData httt.TemplateData
}

func (env *Environment) Load() (err error) {
	if env.Variables == nil {
		env.Variables = make(map[string]string)
	}
	if env.TemplateData == nil {
		env.TemplateData = httt.TemplateData{}
	}
	for _, e := range os.Environ() {
		kv := strings.SplitN(e, "=", 2)
		env.Variables[kv[0]] = kv[1]
	}
	for k, v := range env.Variables {
		var path []string
		if path = env.ParseKeyPath(k); path == nil {
			continue
		}
		env.TemplateData.Set(strings.Join(path, "."), v)
	}
	return nil
}

// ParseKeyPath parses HT_* environment variable names into template data key paths.
func (env *Environment) ParseKeyPath(key string) (path []string) {
	if !strings.HasPrefix(key, "HT_") {
		return nil
	}
	return strings.Split(strings.TrimPrefix(key, "HT_"), "_")
}
