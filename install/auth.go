package install

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/ipfans/my-pi-package/settings"
)

var authEnvVars = []struct {
	env, provider string
}{
	{"ANTHROPIC_API_KEY", "anthropic"},
	{"OPENAI_API_KEY", "openai"},
	{"GOOGLE_API_KEY", "google"},
	{"GEMINI_API_KEY", "google"},
	{"OPENROUTER_API_KEY", "openrouter"},
	{"TOGETHER_API_KEY", "together"},
	{"GROQ_API_KEY", "groq"},
	{"MISTRAL_API_KEY", "mistral"},
}

type authState struct {
	envProviders  []string // "provider (ENV)"
	fileProviders []string
	path          string
	authed        bool
}

func detectAuth(paths settings.Paths) authState {
	st := authState{path: paths.AuthPath()}
	seen := map[string]struct{}{}
	for _, pair := range authEnvVars {
		if os.Getenv(pair.env) == "" {
			continue
		}
		if _, ok := seen[pair.provider]; ok {
			continue
		}
		seen[pair.provider] = struct{}{}
		st.envProviders = append(st.envProviders, pair.provider+" ("+pair.env+")")
	}
	if data, err := os.ReadFile(st.path); err == nil {
		var m map[string]any
		if json.Unmarshal(data, &m) == nil {
			for k := range m {
				st.fileProviders = append(st.fileProviders, k)
			}
		}
	}
	st.authed = len(st.envProviders) > 0 || len(st.fileProviders) > 0
	return st
}

func formatAuth(st authState) string {
	var bits []string
	bits = append(bits, st.envProviders...)
	for _, p := range st.fileProviders {
		bits = append(bits, p+" (auth.json)")
	}
	if len(bits) == 0 {
		return "none detected"
	}
	return strings.Join(bits, ", ")
}
