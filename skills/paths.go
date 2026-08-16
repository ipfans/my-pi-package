package skills

import (
	"os"
	"path/filepath"

	"github.com/ipfans/my-pi-package/settings"
)

// Dir returns the skills directory for the given scope.
// Global: <agentDir>/skills (default ~/.pi/agent/skills)
// Local:  <cwd>/.agents/skills
func Dir(paths settings.Paths) string {
	if paths.Local {
		cwd := paths.Cwd
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		return filepath.Join(cwd, ".agents", "skills")
	}
	return filepath.Join(paths.AgentConfigDir(), "skills")
}
