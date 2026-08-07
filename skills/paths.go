package skills

import (
	"path/filepath"

	"github.com/ipfans/my-pi-package/settings"
)

// Dir returns the Pi skills directory for the given scope.
// Global: <agentDir>/skills (default ~/.pi/agent/skills)
// Local:  <cwd>/.pi/skills
func Dir(paths settings.Paths) string {
	if paths.Local {
		return filepath.Join(paths.InstallRoot(), "skills")
	}
	return filepath.Join(paths.AgentConfigDir(), "skills")
}
