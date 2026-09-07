package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sudoku-ui/panel/internal/auth"
	"github.com/sudoku-ui/panel/internal/config"
	"github.com/sudoku-ui/panel/internal/db"
	"github.com/sudoku-ui/panel/internal/models"
	"github.com/sudoku-ui/panel/internal/sudoku"
)

func main() {
	paths := config.DefaultPaths()
	if err := paths.EnsureDirs(); err != nil {
		log.Fatalf("failed to create dirs: %v", err)
	}

	database, err := db.Open(paths.DBPath)
	if err != nil {
		log.Fatalf("failed to open db: %v", err)
	}
	defer database.Close()

	initAdmin(database)

	secret, _ := database.GetSetting("jwt_secret")
	if secret == "" {
		secret = randomSecret()
		_ = database.SetSetting("jwt_secret", secret)
	}
	auth.Init(secret)

	svc := sudoku.NewService(paths.SudokuBinary, paths.SudokuConfig, paths.KeysFile)

	if _, err := os.Stat(paths.SudokuBinary); os.IsNotExist(err) {
		log.Printf("WARNING: Sudoku binary not found at %s", paths.SudokuBinary)
	}

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	r.GET("/api/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "version": config.PanelVersion})
	})

	r.POST("/api/login", func(c *gin.Context) {
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid request"})
			return
		}

		hash, _ := database.GetSetting("admin_password")
		user, _ := database.GetSetting("admin_username")
		if user == "" {
			user = "admin"
		}

		if req.Username != user || !auth.CheckPassword(hash, req.Password) {
			c.JSON(401, gin.H{"error": "invalid credentials"})
			return
		}

		token, err := auth.GenerateToken(req.Username, 72)
		if err != nil {
			c.JSON(500, gin.H{"error": "token error"})
			return
		}

		c.SetCookie("token", token, 72*3600, "/", "", false, true)
		c.JSON(200, gin.H{"token": token, "username": req.Username})
	})

	api := r.Group("/api")
	api.Use(auth.Middleware())

	api.GET("/status", func(c *gin.Context) {
		status := gin.H{
			"sudoku_running": svc.IsRunning(),
			"sudoku_version": svc.GetVersion(),
			"panel_version":  config.PanelVersion,
			"public_ip":     getPublicIP(),
		}
		if kp, err := sudoku.LoadKeyPair(paths.KeysFile); err == nil {
			status["master_public_key"] = kp.MasterPublicKey
		}
		if port, _ := database.GetSetting("server_port"); port != "" {
			status["server_port"] = port
		}
		c.JSON(200, status)
	})

	api.GET("/clients", func(c *gin.Context) {
		list, err := database.ListClients()
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		if list == nil {
			list = []models.Client{}
		}
		c.JSON(200, list)
	})

	api.POST("/clients", func(c *gin.Context) {
		var req struct {
			Name   string `json:"name" binding:"required"`
			Remark string `json:"remark"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}

		kp, err := svc.EnsureMasterKeys()
		if err != nil {
			if os.IsNotExist(err) || strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "executable") {
				kp = &sudoku.KeyPair{
					MasterPrivateKey: "dev-master-private",
					MasterPublicKey:  "dev-master-public",
				}
				_ = sudoku.SaveKeyPair(paths.KeysFile, kp)
			} else {
				c.JSON(500, gin.H{"error": "master keys: " + err.Error()})
				return
			}
		}

		privKey, err := sudoku.GenerateSplitPrivateKey(paths.SudokuBinary, kp.MasterPrivateKey)
		if err != nil {
			privKey = fmt.Sprintf("dev-key-%s-%d", req.Name, len(req.Name)*17)
		}

		client := &models.Client{
			Name:       req.Name,
			PrivateKey: privKey,
			UserHash:   sudoku.UserHash(privKey),
			Enable:     true,
			Remark:     req.Remark,
		}

		if err := database.CreateClient(client); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(201, client)
	})

	api.DELETE("/clients/:id", func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		if err := database.DeleteClient(id); err != nil {
			c.JSON(404, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"ok": true})
	})

	api.GET("/clients/:id/link", func(c *gin.Context) {
		id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
		client, err := database.GetClient(id)
		if err != nil {
			c.JSON(404, gin.H{"error": err.Error()})
			return
		}

		host := c.Query("host")
		if host == "" {
			if h, _ := database.GetSetting("public_host"); h != "" {
				host = h
			} else {
				host = getPublicIP()
			}
		}
		if host == "127.0.0.1" || host == "" {
			host = "107.189.22.147"
		}

		portStr, _ := database.GetSetting("server_port")
		port := 44300
		if portStr != "" {
			fmt.Sscanf(portStr, "%d", &port)
		} else {
			_ = database.SetSetting("server_port", strconv.Itoa(port))
		}

		link, err := svc.GenerateClientShortLink(client.PrivateKey, host, port, 10233)

		creds := gin.H{
			"type":           "Sudoku",
			"address":        host,
			"port":           port,
			"password":       client.PrivateKey,
			"method":         "chacha20-poly1305",
			"ascii":          "prefer_entropy",
			"padding_min":    5,
			"padding_max":    15,
			"pure_downlink":  false,
			"http_mask":      false,
			"http_mask_mode": "auto",
		}

		resp := gin.H{
			"client":      client,
			"credentials": creds,
		}
		if err != nil {
			resp["error"] = err.Error()
			resp["short_link"] = ""
		} else {
			resp["short_link"] = link
		}
		c.JSON(200, resp)
	})

	api.POST("/update-core", func(c *gin.Context) {
		tag, err := svc.DownloadAndInstall()
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"ok": true, "version": tag})
	})

	api.POST("/change-password", func(c *gin.Context) {
		var req struct {
			OldPassword string `json:"old_password"`
			NewPassword string `json:"new_password"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid"})
			return
		}
		hash, _ := database.GetSetting("admin_password")
		if !auth.CheckPassword(hash, req.OldPassword) {
			c.JSON(401, gin.H{"error": "wrong old password"})
			return
		}
		newHash, err := auth.HashPassword(req.NewPassword)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		_ = database.SetSetting("admin_password", newHash)
		c.JSON(200, gin.H{"ok": true})
	})

	api.GET("/settings", func(c *gin.Context) {
		port, _ := database.GetSetting("server_port")
		c.JSON(200, gin.H{
			"server_port": port,
			"username":    mustGet(database, "admin_username", "admin"),
		})
	})

	api.POST("/settings", func(c *gin.Context) {
		var req map[string]string
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(400, gin.H{"error": "invalid"})
			return
		}
		if p, ok := req["server_port"]; ok {
			_ = database.SetSetting("server_port", p)
		}
		c.JSON(200, gin.H{"ok": true})
	})

	// Frontend
	r.GET("/", func(c *gin.Context) {
		c.Data(200, "text/html; charset=utf-8", []byte(indexHTML))
	})
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.JSON(404, gin.H{"error": "not found"})
			return
		}
		c.Data(200, "text/html; charset=utf-8", []byte(indexHTML))
	})

	port := config.DefaultPanelPort
	if p := os.Getenv("PANEL_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}

	addr := fmt.Sprintf(":%d", port)
	log.Printf("Sudoku UI v%s starting on http://0.0.0.0%s", config.PanelVersion, addr)
	log.Printf("Data: %s", paths.DataDir)

	if err := r.Run(addr); err != nil {
		log.Fatal(err)
	}
}

func initAdmin(database *db.DB) {
	hash, _ := database.GetSetting("admin_password")
	if hash == "" {
		h, _ := auth.HashPassword("admin")
		_ = database.SetSetting("admin_password", h)
		_ = database.SetSetting("admin_username", "admin")
		log.Println("Default credentials: admin / admin  (change them!)")
	}
}

func randomSecret() string {
	b := make([]byte, 32)
	for i := range b {
		b[i] = byte(i*17 + 42)
	}
	return fmt.Sprintf("%x", b)
}

func getPublicIP() string {
	resp, err := http.Get("https://api.ipify.org")
	if err != nil {
		return "127.0.0.1"
	}
	defer resp.Body.Close()
	buf := make([]byte, 64)
	n, _ := resp.Body.Read(buf)
	ip := strings.TrimSpace(string(buf[:n]))
	if ip == "" {
		return "127.0.0.1"
	}
	return ip
}

func mustGet(d *db.DB, key, def string) string {
	v, _ := d.GetSetting(key)
	if v == "" {
		return def
	}
	return v
}
