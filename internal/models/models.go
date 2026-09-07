package models

import (
	"time"
)

// Client представляет одного VPN-клиента (инбаунд)
type Client struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`       // Человекочитаемое имя
	PrivateKey string   `json:"private_key"` // Split Private Key
	UserHash  string    `json:"user_hash"`  // sha256(privateKey)[:8] для идентификации
	Enable    bool      `json:"enable"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Remark    string    `json:"remark,omitempty"`
}

// ServerConfig — настройки сервера Sudoku (часть конфига)
type ServerConfig struct {
	Port              int    `json:"port"`
	ASCII             string `json:"ascii"`
	AEAD              string `json:"aead"`
	PaddingMin        int    `json:"padding_min"`
	PaddingMax        int    `json:"padding_max"`
	CustomTable       string `json:"custom_table"`
	EnablePureDownlink bool  `json:"enable_pure_downlink"`
	FallbackAddress   string `json:"fallback_address"`
	SuspiciousAction  string `json:"suspicious_action"`
	HTTPMaskDisable   bool   `json:"http_mask_disable"`
	HTTPMaskMode      string `json:"http_mask_mode"`
	HTTPMaskPathRoot  string `json:"http_mask_path_root"`
}

// PanelSettings — настройки самой панели
type PanelSettings struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"` // bcrypt
	Port         int    `json:"port"`
	Secret       string `json:"secret"` // JWT secret
}

// SystemStatus — статус системы
type SystemStatus struct {
	SudokuRunning   bool   `json:"sudoku_running"`
	SudokuVersion   string `json:"sudoku_version"`
	PanelVersion    string `json:"panel_version"`
	Uptime          string `json:"uptime"`
	PublicIP        string `json:"public_ip"`
	MasterPublicKey string `json:"master_public_key,omitempty"`
}

// CreateClientRequest — запрос на создание клиента
type CreateClientRequest struct {
	Name   string `json:"name" binding:"required"`
	Remark string `json:"remark"`
}

// ClientResponse — ответ с данными клиента + ссылкой
type ClientResponse struct {
	Client
	ShortLink string `json:"short_link"`
	YAML      string `json:"yaml,omitempty"`
}
