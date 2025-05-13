package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

type Selector struct {
	Query string
}

type Resource struct {
	Kind     string
	Selector Selector
}

type Config struct {
	ClusterID           string
	ResyncInterval      uint
	PodConfig           Resource
	SecretConfig        Resource
	NACKAccountConfig   Resource
	DefaultInstallation string
	ValidInstallations  map[string]any
	TowerURL            string
	TowerAPIToken       string
}

// Environment variable names
const (
	EnvClusterID             = "NATS_TOWER_CLUSTER_ID"
	EnvDefaultInstallation   = "NATS_TOWER_DEFAULT_INSTALLATION"
	EnvInstallationsFilePath = "NATS_TOWER_INSTALLATIONS_FILE_PATH"
	EnvTowerURL              = "NATS_TOWER_URL"
	EnvTowerAPITokenPath     = "NATS_TOWER_API_TOKEN_PATH"
	EnvTowerAPIToken         = "NATS_TOWER_API_TOKEN"
	EnvResyncInterval        = "NATS_TOWER_RESYNC_INTERVAL"

	// Pod config
	EnvPodConfigKind     = "NATS_TOWER_POD_CONFIG_KIND"
	EnvPodConfigSelector = "NATS_TOWER_POD_CONFIG_SELECTOR"

	// Secret config
	EnvSecretConfigKind     = "NATS_TOWER_SECRET_CONFIG_KIND"
	EnvSecretConfigSelector = "NATS_TOWER_SECRET_CONFIG_SELECTOR"

	// NACK account config
	EnvNACKConfigKind     = "NATS_TOWER_NACK_CONFIG_KIND"
	EnvNACKConfigSelector = "NATS_TOWER_NACK_CONFIG_SELECTOR"
)

// Default values
const (
	DefaultTowerURL              = ""
	DefaultInstallationsFilePath = "config/installations.yaml"
)

// NewValidInstallationsFromFile reads and parses installations from a YAML file
func NewValidInstallationsFromFile(filepath string) (map[string]any, error) {
	validInstallations := make(map[string]any)
	config, err := os.ReadFile(filepath)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(config, validInstallations)
	if err != nil {
		return nil, err
	}

	return validInstallations, nil
}

// readFileContent reads content from a file path
func readFileContent(path string) (string, error) {
	if path == "" {
		return "", nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// getEnv gets an environment variable or returns the fallback value
func getEnv(key, fallback string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return fallback
}

// NewConfigFromEnv creates a new Config from environment variables
func NewConfigFromEnv() (*Config, error) {
	// Get configurations from environment variables
	clusterID := getEnv(EnvClusterID, "")
	defaultInstallation := getEnv(EnvDefaultInstallation, "")
	installationsFilePath := getEnv(EnvInstallationsFilePath, DefaultInstallationsFilePath)
	towerURL := getEnv(EnvTowerURL, DefaultTowerURL)

	// Parse resync interval
	var resyncInterval uint
	if resyncStr := getEnv(EnvResyncInterval, "0"); resyncStr != "" {
		if val, err := strconv.ParseUint(resyncStr, 10, 64); err == nil {
			resyncInterval = uint(val)
		} else {
			return nil, fmt.Errorf("invalid resync interval format: %s", err.Error())
		}
	}

	// Handle API token from file or environment
	var towerAPIToken string
	if tokenPath := getEnv(EnvTowerAPITokenPath, ""); tokenPath != "" {
		tokenContent, err := readFileContent(tokenPath)
		if err != nil {
			return nil, fmt.Errorf("error reading API token from file: %s", err.Error())
		}
		towerAPIToken = tokenContent
	} else {
		towerAPIToken = getEnv(EnvTowerAPIToken, "")
	}

	// Validate required fields
	if clusterID == "" {
		return nil, fmt.Errorf("cluster ID is required: set %s environment variable", EnvClusterID)
	}

	if towerAPIToken == "" {
		return nil, fmt.Errorf("tower API token is required: set %s environment variable or provide a token file path with %s",
			EnvTowerAPIToken, EnvTowerAPITokenPath)
	}

	// Load valid installations
	validInstallations, err := NewValidInstallationsFromFile(installationsFilePath)
	if err != nil {
		return nil, fmt.Errorf("error loading valid installations: %s", err.Error())
	}

	// Create resource configurations
	podConfig := Resource{
		Kind: getEnv(EnvPodConfigKind, ""),
		Selector: Selector{
			Query: getEnv(EnvPodConfigSelector, ""),
		},
	}

	secretConfig := Resource{
		Kind: getEnv(EnvSecretConfigKind, ""),
		Selector: Selector{
			Query: getEnv(EnvSecretConfigSelector, ""),
		},
	}

	nackAccountConfig := Resource{
		Kind: getEnv(EnvNACKConfigKind, ""),
		Selector: Selector{
			Query: getEnv(EnvNACKConfigSelector, ""),
		},
	}

	return &Config{
		ClusterID:           clusterID,
		DefaultInstallation: defaultInstallation,
		ResyncInterval:      resyncInterval,
		PodConfig:           podConfig,
		SecretConfig:        secretConfig,
		NACKAccountConfig:   nackAccountConfig,
		ValidInstallations:  validInstallations,
		TowerURL:            towerURL,
		TowerAPIToken:       towerAPIToken,
	}, nil
}
