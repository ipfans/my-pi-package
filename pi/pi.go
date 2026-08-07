// Package pi runs the pi / npm / bun tooling used by my-pi-package.
package pi

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// HasCommand reports whether name is on PATH.
func HasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// Run inherits stdio and returns the process exit code.
func Run(name string, args []string, env []string) int {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if len(env) > 0 {
		cmd.Env = env
	}
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "failed to run %s: %v\n", name, err)
		return 1
	}
	return 0
}

// RunOutput runs a command and returns combined stdout/stderr text.
func RunOutput(name string, args []string) (string, int) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return string(out), ee.ExitCode()
		}
		return string(out), 1
	}
	return string(out), 0
}

// InstallPiGlobal runs npm install -g for the pi core package spec.
func InstallPiGlobal(spec string) int {
	return Run("npm", []string{"install", "-g", spec}, nil)
}

// InstallPackage runs pi install [ -l ] source.
func InstallPackage(source string, local bool, gitSource bool) int {
	args := []string{"install"}
	if local {
		args = append(args, "-l")
	}
	args = append(args, source)
	var env []string
	if gitSource {
		env = append(os.Environ(), "npm_config_ignore_scripts=true")
	}
	return Run("pi", args, env)
}

// RemovePackage runs pi remove [ -l ] source.
func RemovePackage(source string, local bool) int {
	args := []string{"remove"}
	if local {
		args = append(args, "-l")
	}
	args = append(args, source)
	return Run("pi", args, nil)
}

// Update runs pi update or pi update --extensions for local scope.
func Update(local bool) int {
	if local {
		return Run("pi", []string{"update", "--extensions"}, nil)
	}
	return Run("pi", []string{"update"}, nil)
}

// Version returns pi --version output (trimmed).
func Version() string {
	out, code := RunOutput("pi", []string{"--version"})
	if code != 0 {
		return ""
	}
	return strings.TrimSpace(out)
}
