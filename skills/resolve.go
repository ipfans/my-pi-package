package skills

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Repo is an opened marketplace tree (local path or temporary clone).
type Repo struct {
	Root      string
	Cleanup   func() // may be nil
	SourceURL string // human-readable source for logging
}

// Close removes a temporary clone if any.
func (r *Repo) Close() {
	if r != nil && r.Cleanup != nil {
		r.Cleanup()
		r.Cleanup = nil
	}
}

// githubShorthand matches owner/repo (no extra slashes, no leading dot).
var githubShorthand = regexp.MustCompile(`^([A-Za-z0-9_.-]+)/([A-Za-z0-9_.-]+)$`)

// OpenRepo resolves source to a local marketplace root.
// source may be a local directory, owner/repo shorthand, or git URL.
func OpenRepo(source string) (*Repo, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil, fmt.Errorf("source is required")
	}

	// Local directory
	if st, err := os.Stat(source); err == nil && st.IsDir() {
		abs, err := filepath.Abs(source)
		if err != nil {
			return nil, fmt.Errorf("resolving path: %w", err)
		}
		return &Repo{Root: abs, SourceURL: abs}, nil
	}

	url, err := cloneURL(source)
	if err != nil {
		return nil, err
	}
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git is not on PATH (required to clone %s)", url)
	}

	tmp, err := os.MkdirTemp("", "my-pi-package-skills-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp dir: %w", err)
	}
	cleanup := func() { _ = os.RemoveAll(tmp) }

	cmd := exec.Command("git", "clone", "--depth", "1", url, tmp)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		cleanup()
		return nil, fmt.Errorf("git clone %s: %w", url, err)
	}
	return &Repo{Root: tmp, Cleanup: cleanup, SourceURL: url}, nil
}

// cloneURL turns a user source string into a git clone URL.
func cloneURL(source string) (string, error) {
	if isRemoteURL(source) {
		return source, nil
	}
	// owner/repo shorthand → github.com
	if m := githubShorthand.FindStringSubmatch(source); m != nil {
		return fmt.Sprintf("https://github.com/%s/%s.git", m[1], m[2]), nil
	}
	// bare github.com/owner/repo without scheme
	if strings.HasPrefix(source, "github.com/") {
		return "https://" + source, nil
	}
	return "", fmt.Errorf("unrecognized source %q (use owner/repo, git URL, or local path)", source)
}
