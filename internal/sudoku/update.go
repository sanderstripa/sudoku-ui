package sudoku

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const githubRepo = "SUDOKU-ASCII/sudoku"

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// GetLatestVersion возвращает последний тег с GitHub
func (s *Service) GetLatestVersion() (string, error) {
	resp, err := http.Get("https://api.github.com/repos/" + githubRepo + "/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("github api status %d", resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	return strings.TrimPrefix(rel.TagName, "v"), nil
}

// DownloadAndInstall скачивает и устанавливает последнюю версию ядра
func (s *Service) DownloadAndInstall() (string, error) {
	resp, err := http.Get("https://api.github.com/repos/" + githubRepo + "/releases/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}

	arch := runtime.GOARCH
	if arch == "amd64" {
		arch = "amd64"
	} else if arch == "arm64" {
		arch = "arm64"
	} else {
		return "", fmt.Errorf("unsupported arch: %s", arch)
	}

	// Ищем подходящий ассет (обычно sudoku_linux_amd64 или похожее)
	var downloadURL, assetName string
	for _, a := range rel.Assets {
		name := strings.ToLower(a.Name)
		if strings.Contains(name, "linux") && strings.Contains(name, arch) {
			downloadURL = a.BrowserDownloadURL
			assetName = a.Name
			break
		}
	}

	// Fallback — ищем просто бинарник
	if downloadURL == "" {
		for _, a := range rel.Assets {
			name := strings.ToLower(a.Name)
			if !strings.Contains(name, ".tar") && !strings.Contains(name, ".zip") &&
				!strings.Contains(name, "darwin") && !strings.Contains(name, "windows") &&
				!strings.Contains(name, "android") {
				downloadURL = a.BrowserDownloadURL
				assetName = a.Name
				break
			}
		}
	}

	if downloadURL == "" {
		return "", fmt.Errorf("no suitable asset found in release %s", rel.TagName)
	}

	// Скачиваем во временный файл
	tmpDir, err := os.MkdirTemp("", "sudoku-update-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	tmpFile := filepath.Join(tmpDir, assetName)
	if err := downloadFile(downloadURL, tmpFile); err != nil {
		return "", err
	}

	// Если это архив — распаковываем
	var binaryPath string
	if strings.HasSuffix(assetName, ".tar.gz") || strings.HasSuffix(assetName, ".tgz") {
		cmd := exec.Command("tar", "-xzf", tmpFile, "-C", tmpDir)
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("tar extract failed: %w", err)
		}
		// Ищем бинарник
		entries, _ := os.ReadDir(tmpDir)
		for _, e := range entries {
			if !e.IsDir() && (e.Name() == "sudoku" || strings.HasPrefix(e.Name(), "sudoku")) {
				binaryPath = filepath.Join(tmpDir, e.Name())
				break
			}
		}
	} else {
		binaryPath = tmpFile
	}

	if binaryPath == "" {
		return "", fmt.Errorf("binary not found after download")
	}

	// Делаем исполняемым и копируем на место
	os.Chmod(binaryPath, 0755)

	if err := os.MkdirAll(filepath.Dir(s.BinaryPath), 0755); err != nil {
		return "", err
	}

	// Атомарная замена
	targetTmp := s.BinaryPath + ".new"
	data, err := os.ReadFile(binaryPath)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(targetTmp, data, 0755); err != nil {
		return "", err
	}
	if err := os.Rename(targetTmp, s.BinaryPath); err != nil {
		return "", err
	}

	// Пытаемся перезапустить сервис
	_ = exec.Command("systemctl", "restart", "sudoku").Run()

	return rel.TagName, nil
}

func downloadFile(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download status %d", resp.StatusCode)
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
