package hypertexting

import (
	"os"
	"strings"

	hypertemplates "github.com/herdworks/hypertemplates-go"
)

type Environment struct {
	Variables map[string]string
	TemplateData hypertemplates.TemplateData
}

func (env *Environment) Load() (err error) {
	if env.Variables == nil { env.Variables = make(map[string]string) }
	if env.TemplateData == nil { env.TemplateData = hypertemplates.TemplateData{} }
	for _, e := range os.Environ() {
        kv := strings.SplitN(e, "=", 2)
        env.Variables[kv[0]] = kv[1]
    }
	for k, v := range env.Variables {
		var path []string
		if path = env.ParseKeyPath(k); path == nil { continue }
		env.TemplateData.SetPathValue(path, v)
	}
	return nil
}

func (env *Environment) ParseKeyPath(key string) (path []string) {
	if !strings.HasPrefix(key, "HT_") { return nil }
	return strings.Split(strings.TrimPrefix(key, "HT_"), "_")
}
