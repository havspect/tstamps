package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/havspect/tstamps/internal/client"
)

// clientConfigRel is the config file under the user's home directory.
var clientConfigRel = filepath.Join(".config", "tstamps", "config.json")

// LoadClient resolves the client settings: flags, then TSTAMPS_URL/TSTAMPS_TOKEN, then
// the config file. A missing file is not an error.
func LoadClient(urlFlag, tokenFlag string, getenv func(string) string) (client.Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return client.Config{}, err
	}
	return loadClient(urlFlag, tokenFlag, getenv, home)
}

// SaveClient writes cfg to the config file with mode 0600.
func SaveClient(cfg client.Config) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	return saveClient(cfg, home)
}

func loadClient(urlFlag, tokenFlag string, getenv func(string) string, home string) (client.Config, error) {
	cfg, err := readClient(home)
	if err != nil {
		return client.Config{}, err
	}
	if v := getenv("TSTAMPS_URL"); v != "" {
		cfg.URL = v
	}
	if v := getenv("TSTAMPS_TOKEN"); v != "" {
		cfg.Token = v
		cfg.RefreshToken = ""
	}
	if urlFlag != "" {
		cfg.URL = urlFlag
	}
	if tokenFlag != "" {
		cfg.Token = tokenFlag
		cfg.RefreshToken = ""
	}
	return cfg, nil
}

// readClient returns the saved settings, or a zero config when the file is absent.
func readClient(home string) (client.Config, error) {
	b, err := os.ReadFile(filepath.Join(home, clientConfigRel))
	if errors.Is(err, fs.ErrNotExist) {
		return client.Config{}, nil
	}
	if err != nil {
		return client.Config{}, err
	}
	var cfg client.Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return client.Config{}, err
	}
	return cfg, nil
}

func saveClient(cfg client.Config, home string) error {
	path := filepath.Join(home, clientConfigRel)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o600); err != nil {
		return err
	}
	// WriteFile keeps the mode of a file that already exists, and this one holds a token.
	return os.Chmod(path, 0o600)
}
