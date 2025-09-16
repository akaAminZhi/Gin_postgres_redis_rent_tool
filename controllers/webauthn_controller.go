package controllers

import (
	"Gin_postgres_redis_rent_tool/app"
	"Gin_postgres_redis_rent_tool/db"
	"Gin_postgres_redis_rent_tool/models"
	"Gin_postgres_redis_rent_tool/session"
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

type srv struct {
	wa   *webauthn.WebAuthn
	Repo *db.Repo
	sess *session.Store

	appSess   *session.AppSessionStore // ✅ 业务会话（登录态）用
	webOrigin string
}

func (s *srv) AppSessions() *session.AppSessionStore {
	return s.appSess
}
func GetSrv(a *app.App) *srv {
	s := &srv{
		wa:        a.WA,
		Repo:      db.NewRepo(a.DB),
		sess:      session.NewStore(a.RDB, a.Config.SessionTTL),
		appSess:   session.NewAppSessionStore(a.RDB, 1*24*time.Hour),
		webOrigin: a.Config.WebOrigin, // 可选
	}
	return s
}

func (s *srv) GetAppSess() *session.AppSessionStore {
	return s.appSess
}

// webauthn.User 的实现包装：从 DB 模型构造
// 这里只把必要字段映射到库需要的结构

type waUser struct {
	user  models.User
	creds []webauthn.Credential
}

func (u *waUser) WebAuthnID() []byte                         { id, _ := uuid.Parse(u.user.ID); return id[:] }
func (u *waUser) WebAuthnName() string                       { return u.user.Username }
func (u *waUser) WebAuthnDisplayName() string                { return u.user.DisplayName }
func (u *waUser) WebAuthnIcon() string                       { return "" }
func (u *waUser) WebAuthnCredentials() []webauthn.Credential { return u.creds }

func toWaCred(c models.Credential) webauthn.Credential {
	return webauthn.Credential{
		ID:              c.CredentialID,
		PublicKey:       c.PublicKey,
		AttestationType: c.AttestationType,
		Authenticator: webauthn.Authenticator{
			AAGUID:       c.AAGUID,
			SignCount:    c.SignCount,
			CloneWarning: c.CloneWarning,
		},
		Flags: webauthn.CredentialFlags{
			BackupEligible: c.BackupEligible,
			BackupState:    c.BackupState,
		},
	}
}

// 开始添加：返回注册 options
func (s *srv) BeginAddCredential(c *gin.Context) {
	v, ok := c.Get("userID")
	if !ok {
		c.JSON(401, app.H{"error": "unauthorized get userID error"})
		return
	}
	uid, _ := v.(string)
	ctx := c.Request.Context()

	u, err := s.Repo.FindUserByID(ctx, uid)
	if err != nil {
		c.JSON(401, app.H{"error": "unauthorized not found user"})
		return
	}

	// 载入已有凭据，用于 excludeCredentials，避免重复注册
	cs, _ := s.Repo.LoadUserCredentials(ctx, u.ID)
	waCreds := make([]webauthn.Credential, 0, len(cs))
	for _, cdb := range cs {
		waCreds = append(waCreds, toWaCred(cdb))
	}
	wUser := &waUser{user: *u, creds: waCreds}

	// 推荐：平台优先 + 需要 UV + 可发现凭据（手机 FaceID/TouchID 体验更好）
	opts, sd, err := s.wa.BeginRegistration(
		wUser,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			// AuthenticatorAttachment: protocol.Platform,
			UserVerification: protocol.VerificationRequired,
		}),
	)
	if err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}

	// 用用户名做 key 保存 SessionData
	if err := s.sess.SaveReg(ctx, u.Username, sd); err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}
	c.JSON(200, app.H{"opts": opts}) // 直接返回 options，前端解包 publicKey 层即可
}

// 完成添加：保存新凭据
func (s *srv) FinishAddCredential(c *gin.Context) {
	v, ok := c.Get("userID")
	if !ok {
		c.JSON(401, app.H{"error": "unauthorized"})
		return
	}
	uid, _ := v.(string)
	ctx := c.Request.Context()

	u, err := s.Repo.FindUserByID(ctx, uid)
	if err != nil {
		c.JSON(401, app.H{"error": "unauthorized"})
		return
	}

	cs, _ := s.Repo.LoadUserCredentials(ctx, u.ID)
	waCreds := make([]webauthn.Credential, 0, len(cs))
	for _, cdb := range cs {
		waCreds = append(waCreds, toWaCred(cdb))
	}
	wUser := &waUser{user: *u, creds: waCreds}

	sd, err := s.sess.LoadReg(ctx, u.Username)
	if err != nil {
		c.JSON(400, app.H{"error": "session expired or invalid"})
		return
	}

	cred, err := s.wa.FinishRegistration(wUser, *sd, c.Request)
	if err != nil {
		c.JSON(400, app.H{"error": err.Error()})
		return
	}

	// 保存新凭据
	nc := &models.Credential{
		UserID:          u.ID,
		CredentialID:    cred.ID,
		PublicKey:       cred.PublicKey,
		AttestationType: cred.AttestationType,
		AAGUID:          cred.Authenticator.AAGUID,
		SignCount:       cred.Authenticator.SignCount,
		CloneWarning:    cred.Authenticator.CloneWarning,
		BackupEligible:  cred.Flags.BackupEligible,
		BackupState:     cred.Flags.BackupState,
	}
	if err := s.Repo.AddCredential(ctx, nc); err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}

	s.sess.DelReg(ctx, u.Username)
	c.JSON(200, app.H{"ok": true})
}

// --- Registration ---

func (s *srv) BeginRegistration(c *gin.Context) {
	var req struct {
		InviteToken string `json:"inviteToken" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, app.H{"error": err.Error()})
		fmt.Println(err)
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	// 1) 校验邀请
	inv, err := s.Repo.GetInviteByToken(ctx, req.InviteToken)
	if err != nil || inv.UsedAt != nil || time.Now().After(inv.ExpiresAt) {
		c.JSON(403, app.H{"error": "invalid or expired invite"})
		return
	}

	// 2) 用户名 = 邀请邮箱（强制）
	// 若不存在则创建用户（UUID）
	username := inv.Email
	nid := uuid.NewString()
	u, err := s.Repo.FindOrCreateUser(ctx, username, nid)
	if err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}
	// fmt.Println("haha")
	// 载入该用户已有凭据（避免重复注册冲突可在 allowCredentials/ excludeCredentials 使用）
	cs, _ := s.Repo.LoadUserCredentials(ctx, u.ID)
	waCreds := make([]webauthn.Credential, 0, len(cs))
	for _, cdb := range cs {
		waCreds = append(waCreds, toWaCred(cdb))
	}
	wUser := &waUser{user: *u, creds: waCreds}
	// fmt.Println("haha2")

	// 用 RegistrationOption 设定注册时的 UV 与 RK 要求
	opts, sd, err := s.wa.BeginRegistration(
		wUser,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			// AuthenticatorAttachment: protocol.AuthenticatorAttachmentPlatform, // 可选
			UserVerification: protocol.VerificationRequired, // 关键：注册期的 UV 要求
		}),
	)
	// fmt.Println("haha3")

	if err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}
	// fmt.Println("haha4")

	// 5) 会话按 token 存（不是按用户名）
	if err := s.sess.SaveRegByToken(ctx, req.InviteToken, sd); err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}
	// fmt.Println(opts)
	c.JSON(200, app.H{"opts": opts})
}

func (s *srv) FinishRegistration(c *gin.Context) {
	inviteToken := c.Query("inviteToken")
	if inviteToken == "" {
		c.JSON(400, app.H{"error": "missing inviteToken"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	// 1) 取邀请并校验一次性条件
	inv, err := s.Repo.GetInviteByToken(ctx, inviteToken)
	if err != nil || inv.UsedAt != nil || time.Now().After(inv.ExpiresAt) {
		c.JSON(403, app.H{"error": "invalid or expired invite"})
		return
	}
	username := inv.Email
	// 2) 找到该用户（begin 时已创建）
	u, err := s.Repo.FindUserByUsername(ctx, username)
	if err != nil {
		c.JSON(404, app.H{"error": "user not found"})
		return
	}
	// 载入已有凭据（供库校验）
	cs, _ := s.Repo.LoadUserCredentials(ctx, u.ID)
	waCreds := make([]webauthn.Credential, 0, len(cs))
	for _, cdb := range cs {
		waCreds = append(waCreds, toWaCred(cdb))
	}
	wUser := &waUser{user: *u, creds: waCreds}

	// 4) 取注册 SessionData（按 token），并 Finish
	sd, err := s.sess.LoadRegByToken(ctx, inviteToken)
	if err != nil {
		c.JSON(400, app.H{"error": "session expired or invalid"})
		return
	}

	cred, err := s.wa.FinishRegistration(wUser, *sd, c.Request)
	if err != nil {
		c.JSON(400, app.H{"error": err.Error()})
		return
	}

	// 保存凭据
	nc := &models.Credential{
		UserID:          u.ID,
		CredentialID:    cred.ID,
		PublicKey:       cred.PublicKey,
		AttestationType: cred.AttestationType,
		AAGUID:          cred.Authenticator.AAGUID,
		SignCount:       cred.Authenticator.SignCount,
		CloneWarning:    cred.Authenticator.CloneWarning,
		BackupEligible:  cred.Flags.BackupEligible,
		BackupState:     cred.Flags.BackupState,
	}
	if err := s.Repo.AddCredential(ctx, nc); err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}
	// 6) 一次性：会话 & 邀请作废
	s.sess.DelRegByToken(ctx, inviteToken)
	if err := s.Repo.MarkInviteUsed(ctx, inviteToken); err != nil {
		// 可根据需要记录但不影响用户完成
	}

	appSessID := uuid.NewString()
	if err := s.appSess.Create(c.Request.Context(), appSessID, u.ID); err != nil {
		c.JSON(500, app.H{"error": "create app session failed"})
		return
	}

	// 设定 Cookie
	// ✅ 下发 HttpOnly Cookie
	secure := strings.HasPrefix(s.webOrigin, "https://")
	maxAge := int((1 * 24 * time.Hour) / time.Second)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     app.AppSessionCookie,
		Value:    appSessID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode, // 跨站表单少见，Lax 足够；跨域场景改 None+Secure
		Secure:   secure,
		MaxAge:   maxAge,
	})
	ip := c.ClientIP()
	ua := c.Request.UserAgent()
	_ = s.Repo.TouchUserLogin(ctx, u.ID, ip, ua)
	c.JSON(200, app.H{"ok": true, "username": u.Username})
}

// --- Login ---

type loginBeginReq struct {
	Username     string `json:"username"`
	Discoverable bool   `json:"discoverable"`
}

type loginBeginResp struct {
	Options   *protocol.CredentialAssertion `json:"options"`
	SessionID string                        `json:"sessionId"`
}

func (s *srv) BeginLogin(c *gin.Context) {
	var req loginBeginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, app.H{"error": "bad request"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	var (
		opts *protocol.CredentialAssertion
		sd   *webauthn.SessionData
		err  error
	)

	if req.Discoverable {
		opts, sd, err = s.wa.BeginDiscoverableLogin(
			webauthn.WithUserVerification(protocol.VerificationRequired),
		)
		if err != nil {
			c.JSON(500, app.H{"error": err.Error()})
			return
		}
	} else {
		u, err := s.Repo.FindUserByUsername(ctx, req.Username)
		if err != nil {
			c.JSON(404, app.H{"error": "user not found"})
			return
		}
		cs, _ := s.Repo.LoadUserCredentials(ctx, u.ID)
		waCreds := make([]webauthn.Credential, 0, len(cs))
		for _, cdb := range cs {
			waCreds = append(waCreds, toWaCred(cdb))
		}
		wUser := &waUser{user: *u, creds: waCreds}

		opts, sd, err = s.wa.BeginLogin(wUser, webauthn.WithUserVerification(protocol.VerificationRequired))
		if err != nil {
			c.JSON(500, app.H{"error": err.Error()})
			return
		}
	}

	// 生成会话 ID，存 Redis
	sid := uuid.NewString()
	if err := s.sess.SaveAuth(ctx, sid, sd); err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}
	c.JSON(200, loginBeginResp{Options: opts, SessionID: sid})
}

func (s *srv) FinishLogin(c *gin.Context) {
	sid := c.Query("sessionId")
	if sid == "" {
		c.JSON(400, app.H{"error": "missing sessionId"})
		return
	}
	ip := c.ClientIP()
	ua := c.Request.UserAgent()

	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()
	sd, err := s.sess.LoadAuth(ctx, sid)
	if err != nil {
		c.JSON(400, app.H{"error": "session expired or invalid"})
		return
	}

	username := c.Query("username")
	userID := ""
	if username != "" {
		// 非 discoverable：需要用户上下文
		u, err := s.Repo.FindUserByUsername(ctx, username)
		if err != nil {
			c.JSON(404, app.H{"error": "user not found"})
			return
		}
		userID = u.ID
		cs, _ := s.Repo.LoadUserCredentials(ctx, u.ID)
		waCreds := make([]webauthn.Credential, 0, len(cs))
		for _, cdb := range cs {
			waCreds = append(waCreds, toWaCred(cdb))
		}
		wUser := &waUser{user: *u, creds: waCreds}

		cred, err := s.wa.FinishLogin(wUser, *sd, c.Request)
		if err != nil {
			c.JSON(401, app.H{"error": err.Error()})
			return
		}
		// 更新计数器
		_ = s.Repo.UpdateCredentialCounter(ctx, cred.ID, cred.Authenticator.SignCount, cred.Authenticator.CloneWarning)
		// ✅ 新增：更新用户最近登录 & 凭据最近使用 & 可选事件
		_ = s.Repo.TouchUserLogin(ctx, u.ID, ip, ua)
		_ = s.Repo.TouchCredentialUsed(ctx, cred.ID)
	} else {
		// discoverable：通过 credentialID → user
		handler := func(rawID, userHandle []byte) (webauthn.User, error) {
			u, _, err := s.Repo.FindUserByCredentialID(ctx, rawID)
			if err != nil {
				return nil, protocol.ErrBadRequest.WithDetails("credential not found")
			}
			cs, _ := s.Repo.LoadUserCredentials(ctx, u.ID)
			waCreds := make([]webauthn.Credential, 0, len(cs))
			for _, cdb := range cs {
				waCreds = append(waCreds, toWaCred(cdb))
			}
			return &waUser{user: *u, creds: waCreds}, nil
		}

		user, cred, err := s.wa.FinishPasskeyLogin(handler, *sd, c.Request)
		if err != nil {
			c.JSON(401, app.H{"error": err.Error()})
			return
		}
		userID = user.(*waUser).user.ID
		_ = s.Repo.UpdateCredentialCounter(ctx, cred.ID, cred.Authenticator.SignCount, cred.Authenticator.CloneWarning)
		_ = user                                       // 这里你可以建立 session/cookie
		_ = s.Repo.TouchUserLogin(ctx, userID, ip, ua) // ✅
		_ = s.Repo.TouchCredentialUsed(ctx, cred.ID)   // ✅
	}

	s.sess.DelAuth(ctx, sid)
	// === 新增：创建业务会话并下发 Cookie ===
	appSessID := uuid.NewString()
	if err := s.appSess.Create(c.Request.Context(), appSessID, userID); err != nil {
		c.JSON(500, app.H{"error": "create app session failed"})
		return
	}

	// 设定 Cookie
	// ✅ 下发 HttpOnly Cookie
	secure := strings.HasPrefix(s.webOrigin, "https://")
	maxAge := int((1 * 24 * time.Hour) / time.Second)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     app.AppSessionCookie,
		Value:    appSessID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode, // 跨站表单少见，Lax 足够；跨域场景改 None+Secure
		Secure:   secure,
		MaxAge:   maxAge,
	})

	// 返回前端一个跳转提示（前端拿到就跳转）
	c.JSON(200, app.H{"ok": true, "redirect": "/dashboard"})
}
