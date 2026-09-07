package sudoku

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// IsRunning проверяет, запущен ли процесс (очень простая реализация через pgrep)
func (s *Service) IsRunning() bool {
	cmd := exec.Command("pgrep", "-f", s.BinaryPath)
	err := cmd.Run()
	return err == nil
}

// GetVersion пытается получить версию бинарника
func (s *Service) GetVersion() string {
	if _, err := os.Stat(s.BinaryPath); err != nil {
		return "not installed"
	}
	// У Sudoku пока нет нормального -version, возвращаем "installed"
	return "installed"
}

// EnsureMasterKeys создаёт мастер-ключи, если их ещё нет
func (s *Service) EnsureMasterKeys() (*KeyPair, error) {
	if _, err := os.Stat(s.KeysPath); err == nil {
		return LoadKeyPair(s.KeysPath)
	}

	// Генерируем новые
	kp, err := GenerateMasterKeyPair(s.BinaryPath)
	if err != nil {
		return nil, err
	}

	if err := SaveKeyPair(s.KeysPath, kp); err != nil {
		return nil, err
	}

	return kp, nil
}

// WriteServerConfig записывает конфиг сервера
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

	// Мержим дополнительные опции
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

// GenerateClientShortLink создаёт временный клиентский конфиг и вызывает -export-link
func (s *Service) GenerateClientShortLink(privateKey, serverHost string, serverPort int, clientLocalPort int) (string, error) {
	tmpDir, err := os.MkdirTemp("", "sudoku-link-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	clientCfgPath := filepath.Join(tmpDir, "client.json")

	serverAddress := fmt.Sprintf("%s:%d", serverHost, serverPort)

	clientCfg := map[string]interface{}{
		"mode":                 "client",
		"transport":            "tcp",
		"local_port":           clientLocalPort,
		"server_address":       serverAddress,
		"key":                  privateKey,
		"aead":                 "chacha20-poly1305",
		"ascii":                "prefer_entropy",
		"padding_min":          5,
		"padding_max":          15,
		"enable_pure_downlink": false,
		"httpmask": map[string]interface{}{
			"disable": true, // как в easy-install для клиента
			"mode":    "auto",
		},
		"rule_urls": []string{"global"},
	}

	data, err := json.MarshalIndent(clientCfg, "", "  ")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(clientCfgPath, data, 0644); err != nil {
		return "", err
	}

	cmd := exec.Command(s.BinaryPath, "-c", clientCfgPath, "-export-link")
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Даже при ошибке пробуем вытащить ссылку
		return parseShortLinkFromOutput(string(out)), fmt.Errorf("export-link failed: %w\n%s", err, string(out))
	}

	link := parseShortLinkFromOutput(string(out))
	if link == "" {
		return "", fmt.Errorf("short link not found in output:\n%s", string(out))
	}

	return link, nil
}

func parseShortLinkFromOutput(out string) string {
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "sudoku://") {
			return line
		}
		if strings.Contains(line, "Short link:") {
			parts := strings.SplitN(line, "Short link:", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// DownloadLatestBinary скачивает последний релиз Sudoku (упрощённая заглушка)
func (s *Service) DownloadLatestBinary() error {
	// В реальной реализации здесь будет:
	// 1. Запрос к GitHub API releases
	// 2. Скачивание нужного архива под архитектуру
	// 3. Распаковка и замена бинарника
	// 4. systemctl restart sudoku

	return fmt.Errorf("download not implemented yet — будет в следующем шаге")
}

// Restart перезапускает сервис (через systemctl в проде)
func (s *Service) Restart() error {
	// Для разработки просто ничего не делаем
	// В install.sh будет systemctl restart sudoku
	return nil
}

// Uptime заглушка
func (s *Service) Uptime() string {
	return time.Now().Format(time.RFC3339)
}
