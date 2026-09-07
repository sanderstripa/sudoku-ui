package sudoku

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

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
			"disable": true,
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
	out, _ := cmd.CombinedOutput()
	outStr := string(out)

	if idx := strings.Index(outStr, "sudoku://"); idx >= 0 {
		link := outStr[idx:]
		for i, ch := range link {
			if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '"' {
				link = link[:i]
				break
			}
		}
		if len(link) > 15 {
			return link, nil
		}
	}
	return "", fmt.Errorf("short link not found in output: %s", outStr)
}
