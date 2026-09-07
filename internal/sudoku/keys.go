package sudoku

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// KeyPair хранит мастер-ключи
type KeyPair struct {
	MasterPrivateKey string `json:"master_private_key"`
	MasterPublicKey  string `json:"master_public_key"`
}

// GenerateMasterKeyPair вызывает бинарник sudoku -keygen
func GenerateMasterKeyPair(binaryPath string) (*KeyPair, error) {
	cmd := exec.Command(binaryPath, "-keygen")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("keygen failed: %w\noutput: %s", err, string(out))
	}

	kp := &KeyPair{}
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Master Private Key:") {
			kp.MasterPrivateKey = strings.TrimSpace(strings.TrimPrefix(line, "Master Private Key:"))
		}
		if strings.HasPrefix(line, "Master Public Key:") {
			kp.MasterPublicKey = strings.TrimSpace(strings.TrimPrefix(line, "Master Public Key:"))
		}
		// Также ловим Available Private Key на всякий случай
		if strings.HasPrefix(line, "Available Private Key:") {
			// Не используем как мастер
		}
	}

	if kp.MasterPrivateKey == "" || kp.MasterPublicKey == "" {
		return nil, fmt.Errorf("failed to parse keygen output:\n%s", string(out))
	}

	return kp, nil
}

// GenerateSplitPrivateKey генерирует новый клиентский ключ от мастер-приватного
func GenerateSplitPrivateKey(binaryPath, masterPrivateKey string) (string, error) {
	cmd := exec.Command(binaryPath, "-keygen", "-more", masterPrivateKey)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("split keygen failed: %w\noutput: %s", err, string(out))
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "Split Private Key:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Split Private Key:")), nil
		}
		// Иногда может называться Available Private Key
		if strings.HasPrefix(line, "Available Private Key:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Available Private Key:")), nil
		}
	}

	return "", fmt.Errorf("failed to parse split key from output:\n%s", string(out))
}

// UserHash возвращает короткий идентификатор пользователя (как в API Sudoku)
func UserHash(privateKey string) string {
	h := sha256.Sum256([]byte(privateKey))
	return hex.EncodeToString(h[:8])
}

// SaveKeyPair сохраняет мастер-ключи в файл
func SaveKeyPair(path string, kp *KeyPair) error {
	data, err := json.MarshalIndent(kp, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// LoadKeyPair загружает мастер-ключи
func LoadKeyPair(path string) (*KeyPair, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var kp KeyPair
	if err := json.Unmarshal(data, &kp); err != nil {
		return nil, err
	}
	return &kp, nil
}
