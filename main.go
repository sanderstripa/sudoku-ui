package main

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	Version   = "0.1.0-dev"
	PanelRepo = ""
)

const (
	appDir           = "/etc/sudoku-ui"
	configPath       = appDir + "/config.json"
	statePath        = appDir + "/state.json"
	connDir          = appDir + "/connections"
	coreVersionPath  = appDir + "/core-version"
	sudokuConfigPath = "/etc/sudoku/config.json"
	sudokuBinary     = "/usr/local/bin/sudoku"
	panelBinary      = "/usr/local/bin/sudoku-ui"
)

//go:embed web/*
var webFS embed.FS

type AppConfig struct {
	Username   string `json:"username"`
	Salt       string `json:"salt"`
	PassHash   string `json:"pass_hash"`
	Listen     string `json:"listen"`
	Repo       string `json:"repo,omitempty"`
	PublicPath string `json:"public_path,omitempty"`
	CertFile   string `json:"cert_file,omitempty"`
	KeyFile    string `json:"key_file,omitempty"`
}

type Connection struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Port             int    `json:"port"`
	AEAD             string `json:"aead"`
	TableType        string `json:"table_type"`
	CustomTable      string `json:"custom_table,omitempty"`
	PaddingMin       int    `json:"padding_min"`
	PaddingMax       int    `json:"padding_max"`
	PureDownlink     bool   `json:"pure_downlink"`
	HTTPMask         bool   `json:"http_mask"`
	HTTPMode         string `json:"http_mode,omitempty"`
	Multiplex        string `json:"multiplex,omitempty"`
	MasterPrivate    string `json:"master_private"`
	MasterPublic     string `json:"master_public"`
	AvailablePrivate string `json:"available_private,omitempty"`
	FallbackAddress  string `json:"fallback_address"`
	CreatedAt        string `json:"created_at"`
}

type AccessKey struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	ConnectionID string `json:"connection_id"`
	PrivateKey   string `json:"private_key"`
	CreatedAt    string `json:"created_at"`
}

type State struct {
	Connections []Connection `json:"connections"`
	Keys        []AccessKey  `json:"keys"`
}

type App struct {
	cfg      AppConfig
	mu       sync.Mutex
	state    State
	sessions map[string]session
	logins   map[string][]time.Time
	cpuMu    sync.Mutex
	cpuLast  cpuSnapshot
	netMu    sync.Mutex
	netLast  netSnapshot
}

type session struct {
	Expires time.Time
	CSRF    string
}

type cpuSnapshot struct{ Idle, Total uint64 }
type netSnapshot struct {
	Rx, Tx uint64
	At     time.Time
}

type metricsResponse struct {
	CPUPercent    float64 `json:"cpu_percent"`
	CPUCores      int     `json:"cpu_cores"`
	MemoryPercent float64 `json:"memory_percent"`
	MemoryUsed    uint64  `json:"memory_used"`
	MemoryTotal   uint64  `json:"memory_total"`
	DiskPercent   float64 `json:"disk_percent"`
	DiskUsed      uint64  `json:"disk_used"`
	DiskTotal     uint64  `json:"disk_total"`
	RxRate        float64 `json:"rx_rate"`
	TxRate        float64 `json:"tx_rate"`
	RxTotal       uint64  `json:"rx_total"`
	TxTotal       uint64  `json:"tx_total"`
}

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--init":
			if err := initConfig(os.Args[2:]); err != nil {
				log.Fatal(err)
			}
			return
		case "--install-core":
			if _, _, err := installLatestSudoku(true); err != nil {
				log.Fatal(err)
			}
			return
		case "--version":
			fmt.Println(Version)
			return
		}
	}
	app, err := loadApp()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Sudoku UI %s listening on %s", Version, app.cfg.Listen)
	srv := &http.Server{Addr: app.cfg.Listen, Handler: app.routes(), ReadHeaderTimeout: 10 * time.Second}
	if app.cfg.CertFile != "" && app.cfg.KeyFile != "" {
		log.Fatal(srv.ListenAndServeTLS(app.cfg.CertFile, app.cfg.KeyFile))
	}
	log.Fatal(srv.ListenAndServe())
}

func initConfig(args []string) error {
	var username, password, listen, repo, publicPath, certFile, keyFile string
	listen = ":2095"
	for i := 0; i < len(args); i++ {
		if i+1 >= len(args) {
			break
		}
		switch args[i] {
		case "--username":
			username = args[i+1]
			i++
		case "--password":
			password = args[i+1]
			i++
		case "--listen":
			listen = args[i+1]
			i++
		case "--repo":
			repo = args[i+1]
			i++
		case "--path":
			publicPath = "/" + strings.Trim(args[i+1], "/")
			i++
		case "--cert":
			certFile = args[i+1]
			i++
		case "--key":
			keyFile = args[i+1]
			i++
		}
	}
	if username == "" || password == "" {
		return errors.New("--username and --password are required")
	}
	if err := os.MkdirAll(connDir, 0700); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	cfg := AppConfig{Username: username, PassHash: string(hash), Listen: listen, Repo: repo, PublicPath: publicPath, CertFile: certFile, KeyFile: keyFile}
	if err := writeJSON(configPath, cfg, 0600); err != nil {
		return err
	}
	if _, err := os.Stat(statePath); os.IsNotExist(err) {
		if err := writeJSON(statePath, State{}, 0600); err != nil {
			return err
		}
	}
	return nil
}

func loadApp() (*App, error) {
	var cfg AppConfig
	if err := readJSON(configPath, &cfg); err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var st State
	if err := readJSON(statePath, &st); err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	a := &App{cfg: cfg, state: st, sessions: map[string]session{}, logins: map[string][]time.Time{}}
	a.cpuLast = readCPU()
	a.netLast = readNet()
	return a, nil
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", a.login)
	mux.HandleFunc("/api/logout", a.auth(a.logout))
	mux.HandleFunc("/api/state", a.auth(a.getState))
	mux.HandleFunc("/api/metrics", a.auth(a.getMetrics))
	mux.HandleFunc("/api/logs", a.auth(a.getLogs))
	mux.HandleFunc("/api/connections", a.auth(a.connections))
	mux.HandleFunc("/api/connections/", a.auth(a.connectionAction))
	mux.HandleFunc("/api/keys", a.auth(a.keys))
	mux.HandleFunc("/api/keys/", a.auth(a.keyAction))
	mux.HandleFunc("/api/update/core", a.auth(a.updateCore))
	mux.HandleFunc("/api/update/panel", a.auth(a.updatePanel))
	mux.HandleFunc("/api/updates", a.auth(a.getUpdates))
	sub, _ := fs.Sub(webFS, "web")
	fileServer := http.FileServer(http.FS(sub))
	mux.Handle("/", fileServer)
	handler := securityHeaders(mux)
	if a.cfg.PublicPath == "" || a.cfg.PublicPath == "/" {
		return handler
	}
	root := strings.TrimSuffix(a.cfg.PublicPath, "/")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == root {
			http.Redirect(w, r, root+"/", http.StatusTemporaryRedirect)
			return
		}
		if !strings.HasPrefix(r.URL.Path, root+"/") {
			http.NotFound(w, r)
			return
		}
		r.URL.Path = strings.TrimPrefix(r.URL.Path, root)
		handler.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'")
		next.ServeHTTP(w, r)
	})
}

func (a *App) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie("sudoku_session")
		if err != nil {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		a.mu.Lock()
		s, ok := a.sessions[c.Value]
		if ok && time.Now().After(s.Expires) {
			delete(a.sessions, c.Value)
			ok = false
		}
		a.mu.Unlock()
		if !ok {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(s.CSRF)) != 1 {
				jsonError(w, "CSRF-проверка не пройдена", http.StatusForbidden)
				return
			}
		}
		next(w, r)
	}
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ip := clientIP(r)
	now := time.Now()
	a.mu.Lock()
	recent := a.logins[ip][:0]
	for _, attempt := range a.logins[ip] {
		if now.Sub(attempt) < time.Minute {
			recent = append(recent, attempt)
		}
	}
	if len(recent) >= 5 {
		a.logins[ip] = recent
		a.mu.Unlock()
		jsonError(w, "Слишком много попыток. Повторите через минуту", http.StatusTooManyRequests)
		return
	}
	a.logins[ip] = append(recent, now)
	a.mu.Unlock()
	var in struct{ Username, Password string }
	if json.NewDecoder(r.Body).Decode(&in) != nil {
		jsonError(w, "invalid request", 400)
		return
	}
	uok := subtle.ConstantTimeCompare([]byte(in.Username), []byte(a.cfg.Username)) == 1
	hok := verifyPassword(in.Password, a.cfg)
	if !uok || !hok {
		time.Sleep(250 * time.Millisecond)
		jsonError(w, "Неверный логин или пароль", 401)
		return
	}
	token := randomHex(32)
	csrf := randomHex(32)
	a.mu.Lock()
	a.sessions[token] = session{Expires: time.Now().Add(24 * time.Hour), CSRF: csrf}
	delete(a.logins, ip)
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "sudoku_session", Value: token, Path: "/", HttpOnly: true, Secure: requestIsHTTPS(r), SameSite: http.SameSiteStrictMode, MaxAge: 86400})
	jsonOut(w, map[string]any{"ok": true, "csrf": csrf})
}
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("sudoku_session"); err == nil {
		a.mu.Lock()
		delete(a.sessions, c.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: "sudoku_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	jsonOut(w, map[string]any{"ok": true})
}

func (a *App) getState(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	st := a.state
	a.mu.Unlock()
	type connView struct {
		Connection
		Active bool `json:"active"`
	}
	views := make([]connView, 0, len(st.Connections))
	for _, c := range st.Connections {
		views = append(views, connView{Connection: c, Active: serviceActive(c.ID)})
	}
	// Never expose master private keys to the browser.
	for i := range views {
		views[i].MasterPrivate = ""
		views[i].AvailablePrivate = ""
	}
	type keyView struct{ ID, Name, ConnectionID, CreatedAt string }
	kv := make([]keyView, 0, len(st.Keys))
	for _, k := range st.Keys {
		kv = append(kv, keyView{k.ID, k.Name, k.ConnectionID, k.CreatedAt})
	}
	suggestedPort := 0
	if len(st.Connections) == 0 {
		suggestedPort = randomFreePort(50001, 65535)
	}
	jsonOut(w, map[string]any{
		"version":        Version,
		"panel_version":  displayVersion(Version),
		"core_version":   installedCoreVersion(),
		"connections":    views,
		"keys":           kv,
		"suggested_port": suggestedPort,
		"csrf":           a.sessionCSRF(r),
	})
}

func (a *App) getMetrics(w http.ResponseWriter, r *http.Request) { jsonOut(w, a.metrics()) }

func (a *App) connections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	a.mu.Lock()
	hasConnection := len(a.state.Connections) > 0
	a.mu.Unlock()
	if hasConnection {
		jsonError(w, "На этом сервере уже настроено подключение Sudoku", http.StatusConflict)
		return
	}
	var in struct {
		Name                   string `json:"name"`
		Port                   int    `json:"port"`
		AEAD, TableType        string
		PaddingMin, PaddingMax int
		PureDownlink, HTTPMask bool
		HTTPMode, Multiplex    string
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		jsonError(w, "invalid request", 400)
		return
	}
	if in.Name == "" {
		in.Name = "Main"
	}
	if in.Port < 1 || in.Port > 65535 {
		jsonError(w, "Некорректный порт", 400)
		return
	}
	if portBusy(in.Port) {
		jsonError(w, "Порт уже занят", 409)
		return
	}
	if in.AEAD == "" {
		in.AEAD = "chacha20-poly1305"
	}
	if in.TableType == "" {
		in.TableType = "up_ascii_down_entropy"
	}
	if in.Multiplex == "" {
		in.Multiplex = "off"
	}
	if in.HTTPMode == "" {
		in.HTTPMode = "auto"
	}
	if !validChoice(in.AEAD, "chacha20-poly1305", "aes-128-gcm", "none") || !validChoice(in.TableType, "prefer_entropy", "prefer_ascii", "up_ascii_down_entropy", "up_entropy_down_ascii") || !validChoice(in.Multiplex, "off", "auto", "on") || !validChoice(in.HTTPMode, "legacy", "stream", "poll", "auto", "ws") {
		jsonError(w, "Выбран неподдерживаемый параметр Sudoku", http.StatusBadRequest)
		return
	}
	if in.PaddingMin < 0 || in.PaddingMin > 100 || in.PaddingMax < in.PaddingMin || in.PaddingMax > 100 {
		jsonError(w, "Padding должен удовлетворять условию 0 ≤ минимум ≤ максимум ≤ 100", http.StatusBadRequest)
		return
	}
	available, masterPriv, masterPub, err := generateMasterKeys()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	c := Connection{ID: randomID(), Name: in.Name, Port: in.Port, AEAD: in.AEAD, TableType: in.TableType, PaddingMin: in.PaddingMin, PaddingMax: in.PaddingMax, PureDownlink: in.PureDownlink, HTTPMask: in.HTTPMask, HTTPMode: in.HTTPMode, Multiplex: in.Multiplex, MasterPrivate: masterPriv, MasterPublic: masterPub, AvailablePrivate: available, FallbackAddress: "127.0.0.1:80", CreatedAt: time.Now().Format(time.RFC3339)}
	if err := writeConnectionConfig(c); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if err := serviceEnableStart(c.ID); err != nil {
		_ = os.Remove(sudokuConfigPath)
		jsonError(w, "Не удалось запустить Sudoku: "+err.Error(), 500)
		return
	}
	if !waitForSudoku(c.Port, 4*time.Second) {
		serviceDisableStop(c.ID)
		_ = os.Remove(sudokuConfigPath)
		jsonError(w, "Sudoku не запустился или не открыл выбранный порт. Откройте логи и проверьте конфигурацию Core", http.StatusInternalServerError)
		return
	}
	openFirewall(c.Port)
	a.mu.Lock()
	a.state.Connections = append(a.state.Connections, c)
	err = writeJSON(statePath, a.state, 0600)
	a.mu.Unlock()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "id": c.ID})
}

func (a *App) connectionAction(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/connections/")
	parts := strings.Split(rest, "/")
	id := parts[0]
	if id == "" {
		jsonError(w, "not found", 404)
		return
	}
	switch {
	case r.Method == http.MethodDelete && len(parts) == 1:
		a.mu.Lock()
		idx := -1
		var port int
		for i, c := range a.state.Connections {
			if c.ID == id {
				idx = i
				port = c.Port
				break
			}
		}
		if idx >= 0 {
			a.state.Connections = append(a.state.Connections[:idx], a.state.Connections[idx+1:]...)
			nk := a.state.Keys[:0]
			for _, k := range a.state.Keys {
				if k.ConnectionID != id {
					nk = append(nk, k)
				}
			}
			a.state.Keys = nk
			_ = writeJSON(statePath, a.state, 0600)
		}
		a.mu.Unlock()
		if idx < 0 {
			jsonError(w, "not found", 404)
			return
		}
		serviceDisableStop(id)
		_ = os.Remove(sudokuConfigPath)
		closeFirewall(port)
		jsonOut(w, map[string]any{"ok": true})
	case r.Method == http.MethodPost && len(parts) == 2 && parts[1] == "restart":
		if err := run("systemctl", "restart", "sudoku.service"); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		jsonOut(w, map[string]any{"ok": true})
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (a *App) keys(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var in struct{ Name, ConnectionID string }
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.ConnectionID == "" {
		jsonError(w, "invalid request", 400)
		return
	}
	a.mu.Lock()
	var c *Connection
	connectionIndex := -1
	for i := range a.state.Connections {
		if a.state.Connections[i].ID == in.ConnectionID {
			cc := a.state.Connections[i]
			c = &cc
			connectionIndex = i
			break
		}
	}
	a.mu.Unlock()
	if c == nil {
		jsonError(w, "Подключение не найдено", 404)
		return
	}
	if in.Name == "" {
		in.Name = "Новый ключ"
	}
	priv := c.AvailablePrivate
	var err error
	if priv == "" {
		priv, err = generateSplitKey(c.MasterPrivate)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}
	k := AccessKey{ID: randomID(), Name: in.Name, ConnectionID: c.ID, PrivateKey: priv, CreatedAt: time.Now().Format(time.RFC3339)}
	a.mu.Lock()
	if connectionIndex >= 0 && a.state.Connections[connectionIndex].AvailablePrivate == priv {
		a.state.Connections[connectionIndex].AvailablePrivate = ""
	}
	a.state.Keys = append(a.state.Keys, k)
	err = writeJSON(statePath, a.state, 0600)
	a.mu.Unlock()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOut(w, map[string]any{"ok": true, "id": k.ID})
}

func (a *App) keyAction(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/keys/")
	parts := strings.Split(rest, "/")
	id := parts[0]
	a.mu.Lock()
	var k *AccessKey
	var c *Connection
	for i := range a.state.Keys {
		if a.state.Keys[i].ID == id {
			kk := a.state.Keys[i]
			k = &kk
			break
		}
	}
	if k != nil {
		for i := range a.state.Connections {
			if a.state.Connections[i].ID == k.ConnectionID {
				cc := a.state.Connections[i]
				c = &cc
				break
			}
		}
	}
	a.mu.Unlock()
	if k == nil || c == nil {
		jsonError(w, "not found", 404)
		return
	}
	if r.Method == http.MethodDelete && len(parts) == 1 {
		a.mu.Lock()
		nk := a.state.Keys[:0]
		for _, x := range a.state.Keys {
			if x.ID != id {
				nk = append(nk, x)
			}
		}
		a.state.Keys = nk
		_ = writeJSON(statePath, a.state, 0600)
		a.mu.Unlock()
		jsonOut(w, map[string]any{"ok": true})
		return
	}
	if r.Method == http.MethodPatch && len(parts) == 1 {
		var in struct {
			Name string `json:"name"`
		}
		if json.NewDecoder(r.Body).Decode(&in) != nil || strings.TrimSpace(in.Name) == "" {
			jsonError(w, "Укажите название ключа", http.StatusBadRequest)
			return
		}
		a.mu.Lock()
		for i := range a.state.Keys {
			if a.state.Keys[i].ID == id {
				a.state.Keys[i].Name = strings.TrimSpace(in.Name)
			}
		}
		err := writeJSON(statePath, a.state, 0600)
		a.mu.Unlock()
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOut(w, map[string]any{"ok": true})
		return
	}
	if r.Method != http.MethodGet || len(parts) != 2 {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	host := publicHost(r)
	params := clientParams(*c, *k, host)
	switch parts[1] {
	case "params":
		jsonOut(w, params)
	case "link":
		jsonOut(w, map[string]any{"link": shortLink(params)})
	case "qr":
		link := shortLink(params)
		cmd := exec.Command("qrencode", "-o", "-", "-t", "PNG", "-s", "7", "-m", "2", link)
		out, err := cmd.Output()
		if err != nil {
			jsonError(w, "qrencode не установлен", 500)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(out)
	default:
		jsonError(w, "not found", 404)
	}
}

func (a *App) getLogs(w http.ResponseWriter, r *http.Request) {
	args := []string{"--no-pager", "-n", "250", "-u", "sudoku-ui.service"}
	a.mu.Lock()
	if len(a.state.Connections) > 0 {
		args = append(args, "-u", "sudoku.service")
	}
	a.mu.Unlock()
	out, err := exec.Command("journalctl", args...).CombinedOutput()
	if err != nil && len(out) == 0 {
		out = []byte("Не удалось прочитать журнал: " + err.Error())
	}
	if len(bytes.TrimSpace(out)) == 0 || strings.TrimSpace(string(out)) == "-- No entries --" {
		out = []byte("Записей в журнале пока нет.")
	}
	jsonOut(w, map[string]any{"logs": string(out)})
}

func (a *App) getUpdates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	panelRepo := a.cfg.Repo
	if panelRepo == "" {
		panelRepo = PanelRepo
	}
	panelLatest, panelErr := latestRelease(panelRepo)
	coreLatest, coreErr := latestRelease("SUDOKU-ASCII/sudoku")
	result := map[string]any{
		"panel": map[string]any{"installed": displayVersion(Version), "latest": displayVersion(panelLatest), "available": panelErr == nil && !sameVersion(Version, panelLatest)},
		"core":  map[string]any{"installed": installedCoreVersion(), "latest": displayVersion(coreLatest), "available": coreErr == nil && !sameVersion(installedCoreVersion(), coreLatest)},
	}
	if panelErr != nil {
		result["panel_error"] = panelErr.Error()
	}
	if coreErr != nil {
		result["core_error"] = coreErr.Error()
	}
	jsonOut(w, result)
}

func (a *App) updateCore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	version, updated, err := installLatestSudoku(false)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	a.mu.Lock()
	ids := make([]string, 0, len(a.state.Connections))
	for _, c := range a.state.Connections {
		ids = append(ids, c.ID)
	}
	a.mu.Unlock()
	if updated && len(ids) > 0 {
		_ = run("systemctl", "restart", "sudoku.service")
	}
	message := "Обновлений нет — установлена свежая версия " + version
	if updated {
		message = "Sudoku обновлён до " + version
	}
	jsonOut(w, map[string]any{"ok": true, "updated": updated, "version": version, "message": message})
}

func (a *App) updatePanel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(405)
		return
	}
	repo := a.cfg.Repo
	if repo == "" {
		repo = PanelRepo
	}
	if repo == "" {
		jsonError(w, "Репозиторий панели не задан", 400)
		return
	}
	version, updated, err := installReleaseBinary(repo, "sudoku-ui", panelBinary, Version)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if !updated {
		jsonOut(w, map[string]any{"ok": true, "updated": false, "version": version, "message": "Обновлений нет — установлена свежая версия " + displayVersion(Version)})
		return
	}
	jsonOut(w, map[string]any{"ok": true, "updated": true, "version": version, "message": "Панель обновлена до " + version + ". Служба перезапустится."})
	go func() { time.Sleep(500 * time.Millisecond); _ = run("systemctl", "restart", "sudoku-ui") }()
}

func clientParams(c Connection, k AccessKey, host string) map[string]any {
	return map[string]any{"server": host, "port": c.Port, "key": k.PrivateKey, "aead": c.AEAD, "table_type": c.TableType, "custom_table": c.CustomTable, "padding_min": c.PaddingMin, "padding_max": c.PaddingMax, "pure_downlink": c.PureDownlink, "http_mask": c.HTTPMask, "http_mode": c.HTTPMode, "multiplex": c.Multiplex}
}

// Compatible with the compact sudoku:// format used by the official easy-install/client family.
func shortLink(p map[string]any) string {
	obj := map[string]any{"h": p["server"], "p": p["port"], "k": p["key"], "a": p["table_type"], "e": p["aead"], "m": 10233, "x": !p["pure_downlink"].(bool), "hd": !p["http_mask"].(bool), "hm": p["http_mode"], "mx": p["multiplex"], "hx": "off", "hy": ""}
	if t, ok := p["custom_table"].(string); ok && t != "" {
		obj["t"] = t
	}
	b, _ := json.Marshal(obj)
	return "sudoku://" + base64.RawURLEncoding.EncodeToString(b)
}

func writeConnectionConfig(c Connection) error {
	cfg := map[string]any{
		"mode":                 "server",
		"transport":            "tcp",
		"local_port":           c.Port,
		"fallback_address":     c.FallbackAddress,
		"key":                  c.MasterPublic,
		"aead":                 c.AEAD,
		"suspicious_action":    "fallback",
		"ascii":                c.TableType,
		"padding_min":          c.PaddingMin,
		"padding_max":          c.PaddingMax,
		"enable_pure_downlink": c.PureDownlink,
		"multiplex":            c.Multiplex,
		"custom_tables":        []string{},
		"httpmask": map[string]any{
			"disable":   !c.HTTPMask,
			"mode":      c.HTTPMode,
			"tls":       false,
			"host":      "",
			"path_root": "",
		},
		"reverse": map[string]any{"listen": ""},
	}
	if c.CustomTable != "" {
		cfg["custom_table"] = c.CustomTable
	}
	if err := os.MkdirAll(filepath.Dir(sudokuConfigPath), 0700); err != nil {
		return err
	}
	return writeJSON(sudokuConfigPath, cfg, 0600)
}

func generateMasterKeys() (string, string, string, error) {
	out, err := sudokuKeygen("-keygen")
	if err != nil {
		return "", "", "", err
	}
	s := string(out)
	available := field(s, "Available Private Key:")
	masterPrivate := field(s, "Master Private Key:")
	masterPublic := field(s, "Master Public Key:")
	if available == "" || masterPrivate == "" || masterPublic == "" {
		return "", "", "", errors.New("Sudoku вернул неполный набор ключей")
	}
	return available, masterPrivate, masterPublic, nil
}
func generateSplitKey(master string) (string, error) {
	out, err := sudokuKeygen("-keygen", "-more", master)
	if err != nil {
		return "", err
	}
	v := field(string(out), "Split Private Key:")
	if v == "" {
		return "", errors.New("Sudoku не вернул Split Private Key")
	}
	return v, nil
}

func sudokuKeygen(args ...string) ([]byte, error) {
	out, err := exec.Command(sudokuBinary, args...).CombinedOutput()
	if err == nil {
		return out, nil
	}
	firstErr := commandError("keygen", err, out)
	if _, _, installErr := installLatestSudoku(true); installErr != nil {
		return nil, fmt.Errorf("%v; автоматическое восстановление Sudoku не удалось: %w", firstErr, installErr)
	}
	out, err = exec.Command(sudokuBinary, args...).CombinedOutput()
	if err != nil {
		return nil, commandError("keygen после восстановления", err, out)
	}
	return out, nil
}

func commandError(action string, err error, out []byte) error {
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		detail = err.Error()
	}
	return fmt.Errorf("%s: %s", action, detail)
}
func field(s, prefix string) string {
	for _, l := range strings.Split(s, "\n") {
		if i := strings.Index(l, prefix); i >= 0 {
			return strings.TrimSpace(l[i+len(prefix):])
		}
	}
	return ""
}

func (a *App) metrics() metricsResponse {
	cpu := a.cpuPercent()
	mt, mu := memInfo()
	dt, du := diskInfo("/")
	rx, tx, rr, tr := a.netRates()
	return metricsResponse{CPUPercent: cpu, CPUCores: runtime.NumCPU(), MemoryPercent: pct(mu, mt), MemoryUsed: mu, MemoryTotal: mt, DiskPercent: pct(du, dt), DiskUsed: du, DiskTotal: dt, RxRate: rr, TxRate: tr, RxTotal: rx, TxTotal: tx}
}
func (a *App) cpuPercent() float64 {
	a.cpuMu.Lock()
	defer a.cpuMu.Unlock()
	now := readCPU()
	dT := now.Total - a.cpuLast.Total
	dI := now.Idle - a.cpuLast.Idle
	a.cpuLast = now
	if dT == 0 {
		return 0
	}
	return math.Max(0, math.Min(100, 100*(1-float64(dI)/float64(dT))))
}
func readCPU() cpuSnapshot {
	b, _ := os.ReadFile("/proc/stat")
	f := strings.Fields(strings.SplitN(string(b), "\n", 2)[0])
	var vals []uint64
	for _, x := range f[1:] {
		v, _ := strconv.ParseUint(x, 10, 64)
		vals = append(vals, v)
	}
	var total uint64
	for _, v := range vals {
		total += v
	}
	var idle uint64
	if len(vals) > 3 {
		idle = vals[3]
	}
	if len(vals) > 4 {
		idle += vals[4]
	}
	return cpuSnapshot{idle, total}
}
func memInfo() (uint64, uint64) {
	b, _ := os.ReadFile("/proc/meminfo")
	var total, avail uint64
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(f[1], 10, 64)
		switch strings.TrimSuffix(f[0], ":") {
		case "MemTotal":
			total = v * 1024
		case "MemAvailable":
			avail = v * 1024
		}
	}
	return total, total - avail
}
func readNet() netSnapshot {
	b, _ := os.ReadFile("/proc/net/dev")
	var rx, tx uint64
	for _, l := range strings.Split(string(b), "\n")[2:] {
		p := strings.Split(l, ":")
		if len(p) != 2 {
			continue
		}
		iface := strings.TrimSpace(p[0])
		if iface == "lo" {
			continue
		}
		f := strings.Fields(p[1])
		if len(f) < 9 {
			continue
		}
		r, _ := strconv.ParseUint(f[0], 10, 64)
		t, _ := strconv.ParseUint(f[8], 10, 64)
		rx += r
		tx += t
	}
	return netSnapshot{rx, tx, time.Now()}
}
func (a *App) netRates() (uint64, uint64, float64, float64) {
	a.netMu.Lock()
	defer a.netMu.Unlock()
	n := readNet()
	d := n.At.Sub(a.netLast.At).Seconds()
	var rr, tr float64
	if d > 0 {
		rr = float64(n.Rx-a.netLast.Rx) / d
		tr = float64(n.Tx-a.netLast.Tx) / d
	}
	a.netLast = n
	return n.Rx, n.Tx, rr, tr
}
func pct(used, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(used) / float64(total)
}

func installLatestSudoku(force bool) (string, bool, error) {
	currentVersion := installedCoreVersion()
	if force || currentVersion == "неизвестна" {
		currentVersion = ""
	}
	version, updated, err := installReleaseBinary("SUDOKU-ASCII/sudoku", "sudoku", sudokuBinary, currentVersion)
	if err != nil {
		return "", false, err
	}
	if err := os.MkdirAll(appDir, 0700); err != nil {
		return "", false, err
	}
	if err := os.WriteFile(coreVersionPath, []byte(version+"\n"), 0600); err != nil {
		return "", false, err
	}
	return version, updated, nil
}

func latestRelease(repo string) (string, error) {
	if repo == "" {
		return "", errors.New("репозиторий не задан")
	}
	req, _ := http.NewRequest(http.MethodGet, "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	req.Header.Set("User-Agent", "Sudoku-UI/"+Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return "", errors.New("релизы ещё не опубликованы")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API: %s", resp.Status)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	if rel.TagName == "" {
		return "", errors.New("GitHub вернул релиз без номера версии")
	}
	return rel.TagName, nil
}

func installReleaseBinary(repo, name, dst, currentVersion string) (string, bool, error) {
	api := "https://api.github.com/repos/" + repo + "/releases/latest"
	req, _ := http.NewRequest("GET", api, nil)
	req.Header.Set("User-Agent", "Sudoku-UI/"+Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		if resp.StatusCode == http.StatusNotFound {
			return "", false, errors.New("Для этого проекта ещё не опубликован ни один релиз")
		}
		return "", false, fmt.Errorf("GitHub API: %s", resp.Status)
	}
	var rel struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name               string `json:"name"`
			BrowserDownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", false, err
	}
	if rel.TagName == "" {
		return "", false, errors.New("GitHub вернул релиз без номера версии")
	}
	if currentVersion != "" && sameVersion(currentVersion, rel.TagName) {
		return rel.TagName, false, nil
	}
	arch := runtime.GOARCH
	tokens := []string{arch}
	if arch == "amd64" {
		tokens = append(tokens, "x86_64")
	}
	if arch == "arm64" {
		tokens = append(tokens, "aarch64")
	}
	var url, asset string
	for _, a := range rel.Assets {
		ln := strings.ToLower(a.Name)
		if !strings.Contains(ln, "linux") {
			continue
		}
		ok := false
		for _, t := range tokens {
			if strings.Contains(ln, t) {
				ok = true
			}
		}
		if !ok || strings.Contains(ln, "sha") || strings.HasSuffix(ln, ".txt") {
			continue
		}
		url = a.BrowserDownloadURL
		asset = a.Name
		break
	}
	if url == "" {
		return "", false, fmt.Errorf("Не найден Linux asset для %s/%s", repo, arch)
	}
	tmp, err := os.MkdirTemp("", "sudoku-update-")
	if err != nil {
		return "", false, err
	}
	defer os.RemoveAll(tmp)
	pkg := filepath.Join(tmp, asset)
	if err := download(url, pkg); err != nil {
		return "", false, err
	}
	bin, err := extractBinary(pkg, tmp, name)
	if err != nil {
		return "", false, err
	}
	if err := os.Chmod(bin, 0755); err != nil {
		return "", false, err
	}
	if err := validateReleaseBinary(name, bin); err != nil {
		return "", false, err
	}
	backup := dst + ".bak"
	if _, err := os.Stat(dst); err == nil {
		_ = copyFile(dst, backup)
	}
	if err := copyFile(bin, dst); err != nil {
		return "", false, err
	}
	if err := os.Chmod(dst, 0755); err != nil {
		return "", false, err
	}
	return rel.TagName, true, nil
}
func download(url, dst string) error {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "Sudoku-UI/"+Version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download: %s", resp.Status)
	}
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}
func extractBinary(pkg, tmp, name string) (string, error) {
	ln := strings.ToLower(pkg)
	switch {
	case strings.HasSuffix(ln, ".zip"):
		z, err := zip.OpenReader(pkg)
		if err != nil {
			return "", err
		}
		defer z.Close()
		for _, f := range z.File {
			if filepath.Base(f.Name) == name || strings.HasPrefix(filepath.Base(f.Name), name) {
				rc, _ := f.Open()
				dst := filepath.Join(tmp, name)
				o, _ := os.Create(dst)
				_, e := io.Copy(o, rc)
				o.Close()
				rc.Close()
				return dst, e
			}
		}
	case strings.HasSuffix(ln, ".tar.gz") || strings.HasSuffix(ln, ".tgz"):
		if err := run("tar", "-xzf", pkg, "-C", tmp); err != nil {
			return "", err
		}
	}
	var found string
	filepath.WalkDir(tmp, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Clean(path) == filepath.Clean(pkg) || strings.Contains(path, ".bak") {
			return nil
		}
		base := filepath.Base(path)
		if base == name {
			found = path
			return nil
		}
		if found == "" && strings.HasPrefix(base, name) {
			found = path
		}
		return nil
	})
	if found != "" {
		return found, nil
	} // raw binary fallback
	if fi, err := os.Stat(pkg); err == nil && fi.Size() > 0 {
		return pkg, nil
	}
	return "", errors.New("binary not found in release asset")
}

func validateReleaseBinary(name, path string) error {
	args := []string{"--version"}
	if name == "sudoku" {
		args = []string{"-keygen"}
	}
	out, err := exec.Command(path, args...).CombinedOutput()
	if err != nil {
		return commandError("проверка скачанного "+name, err, out)
	}
	if name == "sudoku" && (field(string(out), "Master Private Key:") == "" || field(string(out), "Master Public Key:") == "") {
		return errors.New("скачанный Sudoku не прошёл проверку keygen")
	}
	return nil
}

func sameVersion(a, b string) bool {
	normalize := func(v string) string { return strings.TrimPrefix(strings.TrimSpace(v), "v") }
	return normalize(a) == normalize(b)
}

func displayVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "неизвестна"
	}
	if strings.HasPrefix(v, "v") {
		return v
	}
	return "v" + v
}

func installedCoreVersion() string {
	b, err := os.ReadFile(coreVersionPath)
	if err != nil || strings.TrimSpace(string(b)) == "" {
		return "неизвестна"
	}
	return displayVersion(string(b))
}

func serviceActive(id string) bool {
	_ = id
	return exec.Command("systemctl", "is-active", "--quiet", "sudoku.service").Run() == nil
}
func serviceEnableStart(id string) error {
	_ = id
	return run("systemctl", "enable", "--now", "sudoku.service")
}
func serviceDisableStop(id string) {
	_ = id
	_ = run("systemctl", "disable", "--now", "sudoku.service")
}
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}
func portBusy(port int) bool {
	l, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		return true
	}
	_ = l.Close()
	return false
}
func waitForSudoku(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if serviceActive("") {
			conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 200*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				return true
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}
func openFirewall(port int) {
	if exec.Command("ufw", "status").Run() == nil {
		_ = run("ufw", "allow", fmt.Sprintf("%d/tcp", port))
	}
}
func closeFirewall(port int) {
	if port > 0 && exec.Command("ufw", "status").Run() == nil {
		_ = run("ufw", "delete", "allow", fmt.Sprintf("%d/tcp", port))
	}
}
func publicHost(r *http.Request) string {
	if h := r.Header.Get("X-Forwarded-Host"); h != "" {
		return strings.Split(h, ":")[0]
	}
	host := strings.Split(r.Host, ":")[0]
	if net.ParseIP(host) != nil && (host == "127.0.0.1" || host == "0.0.0.0") {
		return host
	}
	return host
}
func randomID() string       { return randomHex(6) }
func randomHex(n int) string { b := make([]byte, n); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func validChoice(value string, choices ...string) bool {
	for _, choice := range choices {
		if value == choice {
			return true
		}
	}
	return false
}
func randomFreePort(min, max int) int {
	if min < 1 || max > 65535 || min > max {
		return 0
	}
	b := make([]byte, 2)
	_, _ = rand.Read(b)
	start := min + ((int(b[0])<<8)+int(b[1]))%(max-min+1)
	for i := 0; i <= max-min; i++ {
		port := min + (start-min+i)%(max-min+1)
		if !portBusy(port) {
			return port
		}
	}
	return 0
}
func randomTable() string {
	const chars = "xvp"
	b := make([]byte, 8)
	rb := make([]byte, 8)
	_, _ = rand.Read(rb)
	for i := range b {
		b[i] = chars[int(rb[i])%len(chars)]
	}
	return string(b)
}
func passwordHash(password, salt string) string {
	b := []byte(salt + ":" + password)
	for i := 0; i < 120000; i++ {
		h := sha256.Sum256(b)
		b = h[:]
	}
	return hex.EncodeToString(b)
}

func verifyPassword(password string, cfg AppConfig) bool {
	if strings.HasPrefix(cfg.PassHash, "$2") {
		return bcrypt.CompareHashAndPassword([]byte(cfg.PassHash), []byte(password)) == nil
	}
	// Compatibility with installations made before bcrypt was introduced.
	return subtle.ConstantTimeCompare([]byte(passwordHash(password, cfg.Salt)), []byte(cfg.PassHash)) == 1
}

func clientIP(r *http.Request) string {
	if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); forwarded != "" {
		return forwarded
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

func requestIsHTTPS(r *http.Request) bool {
	return r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

func (a *App) sessionCSRF(r *http.Request) string {
	c, err := r.Cookie("sudoku_session")
	if err != nil {
		return ""
	}
	a.mu.Lock()
	s := a.sessions[c.Value]
	a.mu.Unlock()
	return s.CSRF
}
func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
func writeJSON(path string, v any, mode fs.FileMode) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
func jsonOut(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
func jsonError(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
}

var _ = bytes.MinRead
var _ = sort.Strings
