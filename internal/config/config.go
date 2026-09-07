package config

import (
	"os"
	"path/filepath"
)

const (
	PanelVersion = "0.1.0-dev"
	DefaultPanelPort = 2053
)

// Paths — важные пути на сервере
type Paths struct {
	// Корень данных панели
	DataDir string

	// SQLite база
	DBPath string

	// Бинарник Sudoku
	SudokuBinary string

	// Конфиг Sudoku
	SudokuConfig string

	// Директория конфигов
	SudokuDir string

	// Файл с мастер-ключами
	KeysFile string
}

func DefaultPaths() Paths {
	// В продакшене будет /etc/sudoku-ui и /usr/local/bin/sudoku
	// Для разработки — локальные пути
	base := os.Getenv("SUDOKU_UI_DATA")
	if base == "" {
		base = "./data"
	}

	return Paths{
		DataDir:      base,
		DBPath:       filepath.Join(base, "panel.db"),
		SudokuBinary: filepath.Join(base, "bin", "sudoku"),
		SudokuConfig: filepath.Join(base, "sudoku", "config.json"),
		SudokuDir:    filepath.Join(base, "sudoku"),
		KeysFile:     filepath.Join(base, "sudoku", "keys.json"),
	}
}

func (p Paths) EnsureDirs() error {
	dirs := []string{
		p.DataDir,
		filepath.Dir(p.SudokuBinary),
		p.SudokuDir,
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}
