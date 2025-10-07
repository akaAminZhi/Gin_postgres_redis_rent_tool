// controllers/webauthn_controller.go
package controllers

import (
	"context"
	"time"

	"Gin_postgres_redis_rent_tool/app"
	"Gin_postgres_redis_rent_tool/models"

	"github.com/gin-gonic/gin"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

func (s *Srv) WhoAmI(c *app.Ctx) {
	v, ok := c.Get("userID")
	if !ok {
		c.JSON(401, app.H{"error": "unauthorized"})
		return
	}
	uid, _ := v.(string)

	c.JSON(200, app.H{"userID": uid})
}

// ===== 注册（邀请制） =====

func (s *Srv) BeginRegistration(c *gin.Context) {
	var in struct {
		InviteToken string `json:"inviteToken" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(400, app.H{"error": err.Error()})
		return
	}
	ctx, cancel := context.WithTimeout(c, 3*time.Second)
	defer cancel()

	inv, err := s.Repo.GetInviteByToken(ctx, in.InviteToken)
	if err != nil || inv.UsedAt != nil || time.Now().After(inv.ExpiresAt) {
		c.JSON(403, app.H{"error": "invalid or expired invite"})
		return
	}

	// 若不存在则创建用户（UUID）
	nid := uuid.NewString()
	// 用户名强制 = 邀请邮箱
	u, err := s.Repo.FindOrCreateUser(ctx, inv.Email, nid)
	if err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}

	wUser, _ := s.loadWAUserByID(ctx, u.ID)
	opts, sd, err := s.WA.BeginRegistration(
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

	if err := s.Sess.SaveRegByToken(ctx, in.InviteToken, sd); err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}
	c.JSON(200, app.H{"opts": opts})
}

func (s *Srv) FinishRegistration(c *gin.Context) {
	token := c.Query("inviteToken")
	if token == "" {
		c.JSON(400, app.H{"error": "missing inviteToken"})
		return
	}

	ctx, cancel := context.WithTimeout(c, 3*time.Second)
	defer cancel()
	inv, err := s.Repo.GetInviteByToken(ctx, token)
	if err != nil || inv.UsedAt != nil || time.Now().After(inv.ExpiresAt) {
		c.JSON(403, app.H{"error": "invalid or expired invite"})
		return
	}
	wUser, err := s.loadWAUserByUsername(ctx, inv.Email)
	if err != nil {
		c.JSON(404, app.H{"error": "user not found"})
		return
	}

	sd, err := s.Sess.LoadRegByToken(ctx, token)
	if err != nil {
		c.JSON(400, app.H{"error": "session expired or invalid"})
		return
	}

	cred, err := s.WA.FinishRegistration(wUser, *sd, c.Request)
	if err != nil {
		c.JSON(400, app.H{"error": err.Error()})
		return
	}

	if err := s.Repo.AddCredential(ctx, &models.Credential{
		UserID:          wUser.user.ID,
		CredentialID:    cred.ID,
		PublicKey:       cred.PublicKey,
		AttestationType: cred.AttestationType,
		AAGUID:          cred.Authenticator.AAGUID,
		SignCount:       cred.Authenticator.SignCount,
		CloneWarning:    cred.Authenticator.CloneWarning,
		BackupEligible:  cred.Flags.BackupEligible,
		BackupState:     cred.Flags.BackupState,
	}); err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}
	s.Sess.DelRegByToken(ctx, token)
	_ = s.Repo.MarkInviteUsed(ctx, token)

	// 注册即登录
	if err := s.issueSession(c, c.Writer, wUser.user.ID, c.ClientIP(), c.Request.UserAgent()); err != nil {
		c.JSON(500, app.H{"error": "create app session failed"})
		return
	}
	c.JSON(200, app.H{"ok": true, "username": wUser.user.Username})
}

// ===== 添加新凭据（已登录） =====

func (s *Srv) BeginAddCredential(c *gin.Context) {
	v, ok := c.Get("userID")
	if !ok {
		c.JSON(401, app.H{"error": "unauthorized"})
		return
	}
	uid, _ := v.(string)
	ctx, cancel := context.WithTimeout(c, 3*time.Second)
	defer cancel()

	wUser, err := s.loadWAUserByID(ctx, uid)
	if err != nil {
		c.JSON(401, app.H{"error": "unauthorized"})
		return
	}

	opts, sd, err := s.WA.BeginRegistration(
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

	if err := s.Sess.SaveReg(ctx, wUser.user.Username, sd); err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}
	c.JSON(200, app.H{"opts": opts})
}

func (s *Srv) FinishAddCredential(c *gin.Context) {
	v, ok := c.Get("userID")
	if !ok {
		c.JSON(401, app.H{"error": "unauthorized"})
		return
	}
	uid, _ := v.(string)
	ctx, cancel := context.WithTimeout(c, 3*time.Second)
	defer cancel()

	wUser, err := s.loadWAUserByID(ctx, uid)
	if err != nil {
		c.JSON(401, app.H{"error": "unauthorized"})
		return
	}

	sd, err := s.Sess.LoadReg(ctx, wUser.user.Username)
	if err != nil {
		c.JSON(400, app.H{"error": "session expired or invalid"})
		return
	}

	cred, err := s.WA.FinishRegistration(wUser, *sd, c.Request)
	if err != nil {
		c.JSON(400, app.H{"error": err.Error()})
		return
	}

	if err := s.Repo.AddCredential(ctx, &models.Credential{
		UserID:          wUser.user.ID,
		CredentialID:    cred.ID,
		PublicKey:       cred.PublicKey,
		AttestationType: cred.AttestationType,
		AAGUID:          cred.Authenticator.AAGUID,
		SignCount:       cred.Authenticator.SignCount,
		CloneWarning:    cred.Authenticator.CloneWarning,
		BackupEligible:  cred.Flags.BackupEligible,
		BackupState:     cred.Flags.BackupState,
	}); err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}
	s.Sess.DelReg(ctx, wUser.user.Username)
	c.JSON(200, app.H{"ok": true})
}

// ===== 登录 =====

type loginBeginReq struct {
	Username     string `json:"username"`
	Discoverable bool   `json:"discoverable"`
}
type loginBeginResp struct {
	Options   *protocol.CredentialAssertion `json:"options"`
	SessionID string                        `json:"sessionId"`
}

func (s *Srv) BeginLogin(c *gin.Context) {
	var req loginBeginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, app.H{"error": "bad request"})
		return
	}
	ctx, cancel := context.WithTimeout(c, 3*time.Second)
	defer cancel()

	var (
		opts *protocol.CredentialAssertion
		sd   *webauthn.SessionData
		err  error
	)
	if req.Discoverable {
		opts, sd, err = s.WA.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	} else {
		wUser, err2 := s.loadWAUserByUsername(ctx, req.Username)
		if err2 != nil {
			c.JSON(404, app.H{"error": "user not found"})
			return
		}
		opts, sd, err = s.WA.BeginLogin(wUser, webauthn.WithUserVerification(protocol.VerificationRequired))
	}
	if err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}

	sid := uuid.NewString()
	if err := s.Sess.SaveAuth(ctx, sid, sd); err != nil {
		c.JSON(500, app.H{"error": err.Error()})
		return
	}
	c.JSON(200, loginBeginResp{Options: opts, SessionID: sid})
}

func (s *Srv) FinishLogin(c *gin.Context) {
	sid := c.Query("sessionId")
	if sid == "" {
		c.JSON(400, app.H{"error": "missing sessionId"})
		return
	}
	ip, ua := c.ClientIP(), c.Request.UserAgent()

	ctx, cancel := context.WithTimeout(c, 3*time.Second)
	defer cancel()
	sd, err := s.Sess.LoadAuth(ctx, sid)
	if err != nil {
		c.JSON(400, app.H{"error": "session expired or invalid"})
		return
	}

	var userID string
	if username := c.Query("username"); username != "" {
		wUser, err := s.loadWAUserByUsername(ctx, username)
		if err != nil {
			c.JSON(404, app.H{"error": "user not found"})
			return
		}
		cred, err := s.WA.FinishLogin(wUser, *sd, c.Request)
		if err != nil {
			c.JSON(401, app.H{"error": err.Error()})
			return
		}
		userID = wUser.user.ID
		_ = s.Repo.UpdateCredentialCounter(ctx, cred.ID, cred.Authenticator.SignCount, cred.Authenticator.CloneWarning)
		_ = s.Repo.TouchCredentialUsed(ctx, cred.ID)
	} else {
		handler := func(rawID, _ []byte) (webauthn.User, error) {
			u, _, err := s.Repo.FindUserByCredentialID(ctx, rawID)
			if err != nil {
				return nil, protocol.ErrBadRequest.WithDetails("credential not found")
			}
			w, _ := s.loadWAUserByID(ctx, u.ID)
			return w, nil
		}
		user, cred, err := s.WA.FinishPasskeyLogin(handler, *sd, c.Request)
		if err != nil {
			c.JSON(401, app.H{"error": err.Error()})
			return
		}
		userID = user.(*waUser).user.ID
		_ = s.Repo.UpdateCredentialCounter(ctx, cred.ID, cred.Authenticator.SignCount, cred.Authenticator.CloneWarning)
		_ = s.Repo.TouchCredentialUsed(ctx, cred.ID)
	}
	s.Sess.DelAuth(ctx, sid)

	if err := s.issueSession(ctx, c.Writer, userID, ip, ua); err != nil {
		c.JSON(500, app.H{"error": "create app session failed"})
		return
	}
	c.JSON(200, app.H{"ok": true, "redirect": "/dashboard"})
}
