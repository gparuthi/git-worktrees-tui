package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config maps GitHub owner/repo pairs to a local checkout path. When wtree
// receives a PR URL whose owner/repo matches an entry, it operates on that
// checkout regardless of the current working directory.
type Config struct {
	Repos []RepoConfig `yaml:"repos"`
}

type RepoConfig struct {
	Owner string `yaml:"owner"`
	Repo  string `yaml:"repo"`
	Path  string `yaml:"path"`
}

func configPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "wtree", "repos.yaml")
	}
	return filepath.Join(os.Getenv("HOME"), ".config", "wtree", "repos.yaml")
}

func loadConfig() (*Config, error) {
	path := configPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	// Expand ~ in paths.
	home := os.Getenv("HOME")
	for i, r := range cfg.Repos {
		if strings.HasPrefix(r.Path, "~/") {
			cfg.Repos[i].Path = filepath.Join(home, r.Path[2:])
		}
	}
	return &cfg, nil
}

// findRepoPath returns the configured local path for an owner/repo, or "" if
// not found. Match is case-insensitive.
func (c *Config) findRepoPath(owner, repo string) string {
	owner = strings.ToLower(owner)
	repo = strings.ToLower(repo)
	for _, r := range c.Repos {
		if strings.ToLower(r.Owner) == owner && strings.ToLower(r.Repo) == repo {
			return r.Path
		}
	}
	return ""
}

const sampleConfig = `# wtree repo map — owner/repo -> local checkout path.
# When wtree --from-pr <url> resolves to a PR in one of these repos, it
# creates the worktree adjacent to the configured path, regardless of cwd.
# Paths may start with ~ for $HOME.
#
# Edit this file to add your own repos; the entries below are placeholder
# examples and can be removed.
repos:
  - owner: myorg
    repo: my-monorepo
    path: ~/work/my-monorepo
  - owner: myuser
    repo: my-project
    path: ~/work/my-project
`

// ensureSampleConfig writes a documented stub config the first time wtree
// is run, so users discover the feature.
func ensureSampleConfig() {
	path := configPath()
	if _, err := os.Stat(path); err == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	if err := os.WriteFile(path, []byte(sampleConfig), 0644); err != nil {
		log.Printf("[CONFIG] could not write sample: %v", err)
	}
}
