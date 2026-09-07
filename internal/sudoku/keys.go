package sudoku

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

type KeyPair struct {
	MasterPrivateKey string `json:"master_private_key"`
	MasterPublicKey  string `json:"master_public_key"`
}

func extractKey(line, label string) string {
	idx := strings.Index(strings.ToLower(line), strings.ToLower(label))
	if idx == -1 {
		return ""
	}
	rest := line[idx+len(label):]
	rest = strings.TrimLeft(rest, ": \t")
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	key := fields[0]
	if matched, _ := regexp.MatchString(`^[0-9a-fA-F]{32,}$`, key); matched {
		return key
	}
	return ""
}

func GenerateMasterKeyPair(binaryPath string) (*KeyPair, error) {
	cmd := exec.Command(binaryPath, "-keygen")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("keygen failed: %w\noutput: %s", err, string(out))
	}

	kp := &KeyPair{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if kp.MasterPrivateKey == "" {
			if k := extractKey(line, "Master Private Key"); k != "" {
				kp.MasterPrivateKey = k
			}
		}
		if kp.MasterPublicKey == "" {
			if k := extractKey(line, "Master Public Key"); k != "" {
				kp.MasterPublicKey = k
			}
		}
	}

	if kp.MasterPrivateKey == "" || kp.MasterPublicKey == "" {
		return nil, fmt.Errorf("failed to parse keygen output:\n%s", string(out))
	}

	return kp, nil
}

func GenerateSplitPrivateKey(binaryPath, masterPrivateKey string) (string, error) {
	cmd := exec.Command(binaryPath, "-keygen", "-more", masterPrivateKey)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("split keygen failed: %w\noutput: %s", err, string(out))
	}

	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if k := extractKey(line, "Split Private Key"); k != "" {
			return k, nil
		}
		if k := extractKey(line, "Available Private Key"); k != "" {
			return k, nil
		}
	}

	return "", fmt.Errorf("failed to parse split key from output:\n%s", string(out))
}

func UserHash(privateKey string) string {
	h := sha256.Sum256([]byte(privateKey))
	return hex.EncodeToString(h[:8])
}

func SaveKeyPair(path string, kp *KeyPair) error {
	data, err := json.MarshalIndent(kp, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

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
