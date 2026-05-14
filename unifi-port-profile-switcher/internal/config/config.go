// Package config loads and validates the YAML configuration that holds
// UniFi controller credentials and the set of named port-profile presets.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"runtime"

	"gopkg.in/yaml.v3"
)

// Config is the parsed contents of the YAML config file.
type Config struct {
	Controller ControllerConfig  `yaml:"controller"`
	Presets    map[string]Preset `yaml:"presets"`
	Server     ServerConfig      `yaml:"server"`
}

// ServerConfig holds the HTTP serve subcommand configuration.
type ServerConfig struct {
	Bind      string `yaml:"bind"`
	AuthToken string `yaml:"auth_token"`
}

// ControllerConfig describes how to reach the UniFi OS controller.
type ControllerConfig struct {
	URL         string `yaml:"url"`
	Site        string `yaml:"site"`
	Username    string `yaml:"username"`
	Password    string `yaml:"password"`
	InsecureTLS bool   `yaml:"insecure_tls"`
}

// Preset describes a single named change: on a given switch's port, apply the given port profile.
type Preset struct {
	Switch  string `yaml:"switch"`
	Port    int    `yaml:"port"`
	Profile string `yaml:"profile"`
}

// ErrInsecurePerms is returned when the config file is group- or world-accessible.
var ErrInsecurePerms = errors.New("config file must not be group- or world-readable")

// MissingFieldError reports a required field that was empty after parsing.
type MissingFieldError struct {
	Field string
}

func (e *MissingFieldError) Error() string {
	return "config: missing required field: " + e.Field
}

// Load reads, validates, and returns the config at path.
// On POSIX systems it also rejects configs whose mode bits permit group or other access.
func Load(path string) (*Config, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("config: stat %s: %w", path, err)
	}
	if err := checkPerms(info); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}

	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}

	if c.Controller.Site == "" {
		c.Controller.Site = "default"
	}
	if c.Server.Bind == "" {
		c.Server.Bind = "0.0.0.0:8099"
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) validate() error {
	if c.Controller.URL == "" {
		return &MissingFieldError{Field: "controller.url"}
	}
	if c.Controller.Username == "" {
		return &MissingFieldError{Field: "controller.username"}
	}
	if c.Controller.Password == "" {
		return &MissingFieldError{Field: "controller.password"}
	}
	if len(c.Presets) == 0 {
		return &MissingFieldError{Field: "presets"}
	}
	for name, p := range c.Presets {
		if p.Switch == "" {
			return &MissingFieldError{Field: "presets." + name + ".switch"}
		}
		if p.Port <= 0 {
			return &MissingFieldError{Field: "presets." + name + ".port"}
		}
		if p.Profile == "" {
			return &MissingFieldError{Field: "presets." + name + ".profile"}
		}
	}
	return nil
}

// checkPerms is a no-op on Windows and rejects group/other-readable files on POSIX.
func checkPerms(info fs.FileInfo) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w (got %#o, expected 0600 or 0400)", ErrInsecurePerms, info.Mode().Perm())
	}
	return nil
}
