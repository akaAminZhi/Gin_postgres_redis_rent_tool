// app/bootstrap.go
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"Gin_postgres_redis_rent_tool/db"
)

func BootstrapFirstAdmin(ctx context.Context, cfg Config, repo *db.Repo) {
	fmt.Println("Checking if admin user exists...")
	if cfg.BootstrapEmail == "" {
		return
	}
	// n, _ := repo.CountAdmins(ctx)
	// if n > 0 {
	//     return // 已经有管理员，跳过
	// }

	// 生成一次性邀请
	buf := make([]byte, 16)
	rand.Read(buf)
	token := hex.EncodeToString(buf)

	// CreateInvite(邮箱, token, 过期时间, 邀请人标记)
	if _, err := repo.CreateInvite(ctx, cfg.BootstrapEmail, token, time.Now().Add(24*time.Hour), "bootstrap"); err != nil {
		log.Printf("bootstrap invite failed: %v", err)
		return
	}

	// 打印邀请链接（直接点开注册）
	link := fmt.Sprintf("%s/login?inviteToken=%s", cfg.WebOrigin, token)
	log.Printf("[BOOTSTRAP] No admin found, created an admin invite for %s", cfg.BootstrapEmail)
	log.Printf("[BOOTSTRAP] Open this URL to register the first admin: %s", link)
}
