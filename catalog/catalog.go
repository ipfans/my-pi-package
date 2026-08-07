// Package catalog loads the YAML plugin catalog and resolves install sources.
package catalog

import (
	"bytes"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/ipfans/my-pi-package" // package catalogdata embeds root catalog.yaml
	"gopkg.in/yaml.v3"
)

// PackageType is the install backend for a catalog entry.
type PackageType string

const (
	TypeNPM      PackageType = "npm"
	TypeGit      PackageType = "git"
	TypeCompound PackageType = "compound"
)

// VersionLatest means "track newest" (no pin in the source string).
const VersionLatest = "latest"

// Defaults are catalog-wide defaults.
type Defaults struct {
	Version string `yaml:"version"`
}

// PiCore describes how to install/update the pi coding agent itself.
type PiCore struct {
	Package string `yaml:"package"`
	Version string `yaml:"version"`
}

// Package is one installable catalog entry.
type Package struct {
	ID            string      `yaml:"id"`
	Category      string      `yaml:"category"`
	Type          PackageType `yaml:"type"`
	Package       string      `yaml:"package"` // npm / compound
	Repo          string      `yaml:"repo"`    // git
	Version       string      `yaml:"version"`
	PluginName    string      `yaml:"plugin_name"` // compound only
	DependsOn     []string    `yaml:"depends_on"`
	LegacySources []string    `yaml:"legacy_sources"`
	Description   string      `yaml:"description"`
	Hint          string      `yaml:"hint"`
}

// Catalog is the full plugin directory.
type Catalog struct {
	Defaults   Defaults  `yaml:"defaults"`
	Pi         PiCore    `yaml:"pi"`
	Categories []string  `yaml:"categories"`
	LoadFirst  []string  `yaml:"load_first"`
	Packages   []Package `yaml:"packages"`

	byID map[string]*Package
}

// LoadEmbedded loads the catalog.yaml embedded from the module root
// (catalog.yaml next to go.mod, package catalogdata).
func LoadEmbedded() (*Catalog, error) {
	return Parse(catalogdata.YAML)
}

// LoadFile loads a catalog from disk.
func LoadFile(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading catalog %s: %w", path, err)
	}
	return Parse(data)
}

// Parse decodes and validates a catalog document.
func Parse(data []byte) (*Catalog, error) {
	var c Catalog
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parsing catalog yaml: %w", err)
	}
	if err := c.normalizeAndValidate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Catalog) normalizeAndValidate() error {
	if c.Defaults.Version == "" {
		c.Defaults.Version = VersionLatest
	}
	if c.Pi.Package == "" {
		c.Pi.Package = "@earendil-works/pi-coding-agent"
	}
	if c.Pi.Version == "" {
		c.Pi.Version = VersionLatest
	}
	if len(c.Categories) == 0 {
		return fmt.Errorf("catalog: categories must not be empty")
	}
	if len(c.Packages) == 0 {
		return fmt.Errorf("catalog: packages must not be empty")
	}

	catSet := make(map[string]struct{}, len(c.Categories))
	for _, cat := range c.Categories {
		catSet[cat] = struct{}{}
	}

	c.byID = make(map[string]*Package, len(c.Packages))
	for i := range c.Packages {
		p := &c.Packages[i]
		if p.ID == "" {
			return fmt.Errorf("catalog: package at index %d has empty id", i)
		}
		if _, dup := c.byID[p.ID]; dup {
			return fmt.Errorf("catalog: duplicate package id %q", p.ID)
		}
		if _, ok := catSet[p.Category]; !ok {
			return fmt.Errorf("catalog: package %q has unknown category %q", p.ID, p.Category)
		}
		if p.Version == "" {
			p.Version = c.Defaults.Version
		}
		switch p.Type {
		case TypeNPM, TypeCompound:
			if p.Package == "" {
				return fmt.Errorf("catalog: package %q (type %s) requires package field", p.ID, p.Type)
			}
		case TypeGit:
			if p.Repo == "" {
				return fmt.Errorf("catalog: package %q (type git) requires repo field", p.ID)
			}
		default:
			return fmt.Errorf("catalog: package %q has invalid type %q", p.ID, p.Type)
		}
		if p.Type == TypeCompound && p.PluginName == "" {
			p.PluginName = "compound-engineering"
		}
		c.byID[p.ID] = p
	}

	for _, p := range c.Packages {
		for _, dep := range p.DependsOn {
			if _, ok := c.byID[dep]; !ok {
				return fmt.Errorf("catalog: package %q depends on unknown id %q", p.ID, dep)
			}
		}
	}
	for _, id := range c.LoadFirst {
		if _, ok := c.byID[id]; !ok {
			return fmt.Errorf("catalog: load_first references unknown id %q", id)
		}
	}
	return nil
}

// ByID returns a package by id, or nil.
func (c *Catalog) ByID(id string) *Package {
	return c.byID[id]
}

// AllIDs returns every package id in catalog order.
func (c *Catalog) AllIDs() []string {
	ids := make([]string, 0, len(c.Packages))
	for _, p := range c.Packages {
		ids = append(ids, p.ID)
	}
	return ids
}

// ValidSelector reports whether name is a category or package id.
func (c *Catalog) ValidSelector(name string) bool {
	if slices.Contains(c.Categories, name) {
		return true
	}
	_, ok := c.byID[name]
	return ok
}

// Source resolves the pi install source string for this package.
// version "latest" (or empty) omits the pin; otherwise pins npm@ver or git@ref.
func (p Package) Source() string {
	ver := p.Version
	if ver == "" {
		ver = VersionLatest
	}
	switch p.Type {
	case TypeNPM, TypeCompound:
		if ver == VersionLatest {
			return "npm:" + p.Package
		}
		return "npm:" + p.Package + "@" + ver
	case TypeGit:
		if ver == VersionLatest {
			return "git:" + p.Repo
		}
		return "git:" + p.Repo + "@" + ver
	default:
		return ""
	}
}

// BunxSpec is the package@version argument for bunx (compound only).
func (p Package) BunxSpec() string {
	ver := p.Version
	if ver == "" || ver == VersionLatest {
		return p.Package + "@latest"
	}
	return p.Package + "@" + ver
}

// PiInstallSpec is npm install -g argument for the pi core package.
func (c *Catalog) PiInstallSpec() string {
	if c.Pi.Version == "" || c.Pi.Version == VersionLatest {
		return c.Pi.Package
	}
	return c.Pi.Package + "@" + c.Pi.Version
}

// ResolveSelection returns package ids for --only / --except / default-all.
func (c *Catalog) ResolveSelection(only, except []string) (map[string]struct{}, error) {
	if len(only) > 0 && len(except) > 0 {
		return nil, fmt.Errorf("cannot use --only and --except together")
	}
	if err := c.validateSelectors(only, "--only"); err != nil {
		return nil, err
	}
	if err := c.validateSelectors(except, "--except"); err != nil {
		return nil, err
	}

	selected := make(map[string]struct{})
	switch {
	case len(only) > 0:
		for _, p := range c.Packages {
			if matchesSelector(p, only) {
				selected[p.ID] = struct{}{}
			}
		}
	case len(except) > 0:
		for _, p := range c.Packages {
			if !matchesSelector(p, except) {
				selected[p.ID] = struct{}{}
			}
		}
	default:
		for _, p := range c.Packages {
			selected[p.ID] = struct{}{}
		}
	}
	return selected, nil
}

func (c *Catalog) validateSelectors(list []string, label string) error {
	var bad []string
	for _, name := range list {
		if !c.ValidSelector(name) {
			bad = append(bad, name)
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return fmt.Errorf("unknown %s: %s\nvalid categories: %s\nvalid package ids: %s",
		label,
		strings.Join(bad, ", "),
		strings.Join(c.Categories, ", "),
		strings.Join(c.AllIDs(), ", "),
	)
}

func matchesSelector(p Package, selectors []string) bool {
	for _, s := range selectors {
		if s == p.Category || s == p.ID {
			return true
		}
	}
	return false
}

// ExpandDependsOn closes the selection over depends_on edges.
func (c *Catalog) ExpandDependsOn(selected map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(selected))
	for id := range selected {
		out[id] = struct{}{}
	}
	changed := true
	for changed {
		changed = false
		for id := range out {
			p := c.byID[id]
			if p == nil {
				continue
			}
			for _, dep := range p.DependsOn {
				if _, ok := out[dep]; ok {
					continue
				}
				out[dep] = struct{}{}
				changed = true
			}
		}
	}
	return out
}

// SelectedPackages returns packages whose ids are in selected, in catalog order.
func (c *Catalog) SelectedPackages(selected map[string]struct{}) []Package {
	var out []Package
	for _, p := range c.Packages {
		if _, ok := selected[p.ID]; ok {
			out = append(out, p)
		}
	}
	return out
}

// LoadFirstSources returns resolved sources for load_first packages.
func (c *Catalog) LoadFirstSources() []string {
	var out []string
	for _, id := range c.LoadFirst {
		if p := c.byID[id]; p != nil {
			out = append(out, p.Source())
		}
	}
	return out
}
