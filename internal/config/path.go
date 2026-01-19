package config

import (
	"errors"
	"os"
	"path/filepath"
)

const (
	appDirName  = "ip" // directory name under os.UserConfigDir
	configName  = "config.json"
	stateName   = "state.json"
	defaultBase = "https://www.instapaper.com"
)

func DefaultBaseURL() string { return defaultBase }

func DefaultDir() (string, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if d == "" {
		return "", errors.New("os.UserConfigDir() returned empty string")
	}
	return filepath.Join(d, appDirName), nil
}

func DefaultConfigPath() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, configName), nil
}

func DefaultStatePath() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, stateName), nil
}
