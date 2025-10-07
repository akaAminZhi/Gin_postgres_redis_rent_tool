package controllers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type InviteController struct{ *Srv }

// 用新的 *Srv 作为依赖入口
func GetInviteController(s *Srv) *InviteController { return &InviteController{Srv: s} }

// POST /admin/invites
func (ic *InviteController) CreateInvite(c *gin.Context) {
	var in struct {
		Email   string `json:"email" binding:"required,email"`
		Expires int    `json:"expiresDays"` // 默认 1 天
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if in.Expires <= 0 {
		in.Expires = 1
	}

	// 生成一次性 token
	buf := make([]byte, 16)
	_, _ = rand.Read(buf)
	token := hex.EncodeToString(buf)

	// 落库
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	inv, err := ic.Repo.CreateInvite(
		ctx,
		strings.ToLower(in.Email),
		token,
		time.Now().AddDate(0, 0, in.Expires),
		"admin",
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 拼邀请链接（前端登录页带 inviteToken）
	link := strings.TrimRight(ic.Cfg.WebOrigin, "/") + "/login?inviteToken=" + token

	// 发邮件（若未配置 SMTP，打印日志但不报错）
	if err := ic.sendInviteMail(in.Email, link, in.Expires); err != nil {
		log.Printf("[invite email] send failed: %v", err)
	}

	c.JSON(http.StatusCreated, gin.H{
		"token":  token,
		"link":   link, // 方便开发环境直接点
		"invite": inv,
	})
}

// -------------------- 邮件发送 --------------------

type smtpConf struct {
	Host     string // SMTP_HOST, e.g. smtp.gmail.com
	Port     string // SMTP_PORT, e.g. 587
	Username string // SMTP_USERNAME, e.g. your@gmail.com
	Password string // SMTP_PASSWORD, app password or smtp password
	From     string // SMTP_FROM,    e.g. no-reply@yourdomain.com (为空时回退 Username)
	AppName  string // APP_NAME,     e.g. Rent Tool
}

func (ic *InviteController) loadSMTP() smtpConf {
	get := func(k, d string) string {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
		return d
	}
	return smtpConf{
		Host:     get("SMTP_HOST", ""),
		Port:     get("SMTP_PORT", "587"),
		Username: get("SMTP_USERNAME", ""),
		Password: get("SMTP_PASSWORD", ""),
		From:     get("SMTP_FROM", ""),
		AppName:  get("APP_NAME", "Rent Tool"),
	}
}

func (ic *InviteController) sendInviteMail(toEmail, link string, expiresDays int) error {
	conf := ic.loadSMTP()

	// 未配置 SMTP → 开发模式：打印即可，不报错
	if conf.Host == "" || (conf.Username == "" && conf.From == "") {
		log.Printf("[DEV] Invite link for %s: %s (expires in %d day(s))", toEmail, link, expiresDays)
		return nil
	}

	fromAddr := conf.From
	if fromAddr == "" {
		fromAddr = conf.Username
	}

	subject := fmt.Sprintf("%s Invitation", conf.AppName)
	htmlBody := fmt.Sprintf(`
<div style="font-family:Arial,sans-serif; font-size:14px; color:#222">
  <p>Hello,</p>
  <p>You have been invited to join <b>%s</b>. Click the button below to create your passkey and sign in:</p>
  <p>
    <a href="%s" style="display:inline-block; padding:10px 16px; background:#2563EB; color:#fff; text-decoration:none; border-radius:6px;">
      Accept Invitation
    </a>
  </p>
  <p>Or open this link directly:</p>
  <p><a href="%s">%s</a></p>
  <p>This invitation will expire in %d day(s).</p>
  <hr/>
  <p style="color:#666">If you did not expect this email, you can safely ignore it.</p>
</div>
`, conf.AppName, link, link, link, expiresDays)

	msg := buildMIMEWithFromName(conf.AppName, fromAddr, toEmail, subject, htmlBody)

	auth := smtp.PlainAuth("", conf.Username, conf.Password, conf.Host)
	addr := conf.Host + ":" + conf.Port
	return smtp.SendMail(addr, auth, fromAddr, []string{toEmail}, []byte(msg))
}

func buildMIMEWithFromName(fromName, fromAddr, to, subject, html string) string {
	headers := []string{
		fmt.Sprintf("From: %s <%s>", fromName, fromAddr),
		fmt.Sprintf("To: %s", to),
		fmt.Sprintf("Subject: %s", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/html; charset=UTF-8",
	}
	return strings.Join(headers, "\r\n") + "\r\n\r\n" + html
}
