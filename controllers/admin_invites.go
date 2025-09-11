package controllers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"time"

	"Gin_postgres_redis_rent_tool/app"
	"Gin_postgres_redis_rent_tool/db"

	"github.com/gin-gonic/gin"
)

type InviteController struct {
	repo     *db.Repo
	smtpHost string
	smtpPort string
}

//	func RegisterInviteAdminRoutes(r *gin.Engine, repo *db.Repo) {
//		ic := &InviteController{repo: repo}
//		r.POST("/admin/invites", ic.CreateInvite)
//	}
func GetInviteController(repo *db.Repo) *InviteController {
	return &InviteController{
		repo:     repo,
		smtpHost: "smtp.gmail.com",
		smtpPort: "587"}
}
func (ic *InviteController) CreateInvite(c *gin.Context) {
	var in struct {
		Email   string `json:"email" binding:"required,email"`
		Expires int    `json:"expiresDays"` // 默认 1 天
	}
	from := "qin.zhimin1991@gmail.com"
	password := "bnri lyor tbzw hika"
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, app.H{"error": err.Error()})
		return
	}
	if in.Expires <= 0 {
		in.Expires = 1
	}
	to := []string{in.Email}
	buf := make([]byte, 16)
	rand.Read(buf)
	token := hex.EncodeToString(buf)

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	inv, err := ic.repo.CreateInvite(ctx, in.Email, token, time.Now().AddDate(0, 0, in.Expires), "admin@example")
	if err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}

	// TODO: 这里发送邮件，把链接发给用户：
	// 例如前端注册页：https://your-app/signup?invite=<token>
	// 邮件内容
	subject := "Subject: This is the rent tool invitation\r\n"
	body := "https://87701380e67f.ngrok-free.app/login?inviteToken=" + token
	message := []byte(subject + "\r\n" + body)
	// 认证
	auth := smtp.PlainAuth("", from, password, ic.smtpHost)

	// 发送邮件
	err = smtp.SendMail(ic.smtpHost+":"+ic.smtpPort, auth, from, to, message)
	if err != nil {
		log.Println(err)
	}
	fmt.Println("Email sent successfully!")
	c.JSON(201, app.H{
		"token":  token,
		"link":   "https://your-app/signup?invite=" + token, // 方便你先手工点
		"invite": inv,
	})
}
