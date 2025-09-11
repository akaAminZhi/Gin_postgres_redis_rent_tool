package app

import (
	"Gin_postgres_redis_rent_tool/db"
	"Gin_postgres_redis_rent_tool/session"
	"context"
	"log"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// 简化别名，便于 handlers 调用
type Ctx = gin.Context
type H = gin.H

// App 聚合各依赖
type App struct {
	Router *gin.Engine
	DB     *gorm.DB
	RDB    *redis.Client
	WA     *webauthn.WebAuthn
	Config Config

	appSess *session.AppSessionStore
}

// Config 从环境变量读取
type Config struct {
	DatabaseURL string
	RedisAddr   string
	RedisPwd    string
	WebOrigin   string
	RPID        string
	RPOrigins   []string
	SessionTTL  time.Duration
	AdminEmails []string
}

func (a *App) AppSessions() *session.AppSessionStore { return a.appSess }

func MustNew() *App {
	cfg := loadConfig()

	// --- DB: Postgres ---
	// dbConn, err := gorm.Open(postgres.Open(cfg.DatabaseURL), &gorm.Config{})
	// if err != nil {
	// 	log.Fatalf("open db: %v", err)
	// }
	// if err := dbConn.AutoMigrate(&db.User{}, &db.Credential{}); err != nil {
	// 	log.Fatalf("migrate: %v", err)
	// }
	dbConn := db.ConnectDB()

	// --- Redis ---
	rdb := redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPwd, DB: 0})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis: %v", err)
	}

	// --- WebAuthn RP ---
	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: "LSB tool Passkeys",
		RPID:          cfg.RPID,
		RPOrigins:     cfg.RPOrigins,
	})
	if err != nil {
		log.Fatalf("webauthn: %v", err)
	}
	// 业务会话：1 天 TTL，可通过环境变量覆盖
	appTTL := 1 * 24 * time.Hour

	// --- Gin ---
	r := gin.Default()
	useCORS(r, cfg.WebOrigin)
	a := &App{
		Router: r, DB: dbConn, RDB: rdb, WA: wa, Config: cfg,
		appSess: session.NewAppSessionStore(rdb, appTTL),
	}
	return a
}

func (a *App) Close() { _ = a.RDB.Close() }

func loadConfig() Config {
	get := func(k, def string) string {
		v := os.Getenv(k)
		if v == "" {
			return def
		}
		return v
	}
	ttlSec := get("SESSION_TTL_SECONDS", "600")
	var ttl time.Duration = 10 * time.Minute
	if d, err := time.ParseDuration(ttlSec + "s"); err == nil {
		ttl = d
	}
	originsCSV := get("RP_ORIGINS", "https://87701380e67f.ngrok-free.app")
	var origins []string
	for _, o := range strings.Split(originsCSV, ",") {
		if s := strings.TrimSpace(o); s != "" {
			origins = append(origins, s)
		}
	}
	adminsCSV := os.Getenv("ADMIN_EMAILS") // 例如: "admin@ex.com,ops@ex.com"
	var admins []string
	for _, s := range strings.Split(adminsCSV, ",") {
		if t := strings.TrimSpace(s); t != "" {
			admins = append(admins, strings.ToLower(t))
		}
	}
	return Config{
		// DatabaseURL: get("DATABASE_URL", "postgres://postgres:postgres@127.0.0.1:5432/webauthn?sslmode=disable"),
		RedisAddr:   get("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPwd:    os.Getenv("REDIS_PASSWORD"),
		WebOrigin:   get("WEB_ORIGIN", "https://87701380e67f.ngrok-free.app"),
		RPID:        get("RP_ID", "87701380e67f.ngrok-free.app"),
		RPOrigins:   origins,
		SessionTTL:  ttl,
		AdminEmails: admins,
	}
}

// 帮助函数：新用户 ID（UUID 字符串 → []byte 作为 userHandle）
func NewUserID() []byte { id := uuid.New(); return id[:] }
