package sudoku

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Service управляет ядром Sudoku
type Service struct {
	BinaryPath string
	ConfigPath string
	KeysPath   string
}

func NewService(binary, config, keys string) *Service {
	return &Service{
		BinaryPath: binary,
		ConfigPath: config,
		KeysPath:   keys,
	}
}

func (s *Service) IsRunning() bool {
	cmd := exec.Command("pgrep", "-f", s.BinaryPath)
	err := cmd.Run()
	return err == nil
}

func (s *Service) GetVersion() string {
	if _, err := os.Stat(s.BinaryPath); err != nil {
		return "not installed"
	}
	return "installed"
}

func (s *Service) EnsureMasterKeys() (*KeyPair, error) {
	if _, err := os.Stat(s.KeysPath); err == nil {
		return LoadKeyPair(s.KeysPath)
	}

	// Если бинарника нет — сразу ошибка, чтобы сработал fallback
	if _, err := os.Stat(s.BinaryPath); err != nil {
		return nil, fmt.Errorf("sudoku binary not found: %w", err)
	}

	kp, err := GenerateMasterKeyPair(s.BinaryPath)
	if err != nil {
		return nil, err
	}

	if err := SaveKeyPair(s.KeysPath, kp); err != nil {
		return nil, err
	}

	return kp, nil
}

func (s *Service) WriteServerConfig(port int, publicKey string, opts map[string]interface{}) error {
	cfg := map[string]interface{}{
		"mode":                 "server",
		"transport":            "tcp",
		"local_port":           port,
		"server_address":       "",
		"fallback_address":     "127.0.0.1:80",
		"key":                  publicKey,
		"aead":                 "chacha20-poly1305",
		"suspicious_action":    "fallback",
		"ascii":                "prefer_entropy",
		"padding_min":          2,
		"padding_max":          7,
		"enable_pure_downlink": false,
		"httpmask": map[string]interface{}{
			"disable": false,
			"mode":    "auto",
		},
	}

	for k, v := range opts {
		cfg[k] = v
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(s.ConfigPath), 0755); err != nil {
		return err
	}

	return os.WriteFile(s.ConfigPath, data, 0644)
}




func (s *Service) Restart() error {
	return nil
}

func (s *Service) Uptime() string {
	return time.Now().Format(time.RFC3339)
}
