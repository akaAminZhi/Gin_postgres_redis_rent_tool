// controllers/srv.go
package controllers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"Gin_postgres_redis_rent_tool/app"
	"Gin_postgres_redis_rent_tool/db"
	"Gin_postgres_redis_rent_tool/models"
	"Gin_postgres_redis_rent_tool/session"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

type Srv struct {
	WA        *webauthn.WebAuthn
	Repo      *db.Repo
	Sess      *session.Store
	AppSess   *session.AppSessionStore
	WebOrigin string
	Cfg       app.Config
}

func GetSrv(a *app.App) *Srv {
	return &Srv{
		WA:        a.WA,
		Repo:      db.NewRepo(a.DB),
		Sess:      session.NewStore(a.RDB, a.Config.SessionTTL),
		AppSess:   session.NewAppSessionStore(a.RDB, 24*time.Hour),
		WebOrigin: a.Config.WebOrigin,
		Cfg:       a.Config,
	}
}

// --- helpers ---

// 统一设置业务会话 Cookie
func (s *Srv) setAppCookie(w http.ResponseWriter, sessionID string, maxAge time.Duration) {
	secure := strings.HasPrefix(s.WebOrigin, "https://")
	http.SetCookie(w, &http.Cookie{
		Name:     app.AppSessionCookie,
		Value:    sessionID,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		MaxAge:   int(maxAge / time.Second),
	})
}

// 登录成功：创建会话 + 触发登录快照
func (s *Srv) issueSession(ctx context.Context, w http.ResponseWriter, userID string, ip, ua string) error {
	if err := s.Repo.TouchUserLogin(ctx, userID, ip, ua); err != nil {
		// 不阻塞
	}
	id := uuid.NewString()
	if err := s.AppSess.Create(ctx, id, userID); err != nil {
		return err
	}
	s.setAppCookie(w, id, 24*time.Hour)
	return nil
}

// WebAuthn: DB user -> waUser
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

func (s *Srv) loadWAUserByID(ctx context.Context, id string) (*waUser, error) {
	u, err := s.Repo.FindUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	cs, _ := s.Repo.LoadUserCredentials(ctx, u.ID)
	ws := make([]webauthn.Credential, 0, len(cs))
	for _, c := range cs {
		ws = append(ws, toWaCred(c))
	}
	return &waUser{user: *u, creds: ws}, nil
}

func (s *Srv) loadWAUserByUsername(ctx context.Context, username string) (*waUser, error) {
	u, err := s.Repo.FindUserByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	cs, _ := s.Repo.LoadUserCredentials(ctx, u.ID)
	ws := make([]webauthn.Credential, 0, len(cs))
	for _, c := range cs {
		ws = append(ws, toWaCred(c))
	}
	return &waUser{user: *u, creds: ws}, nil
}
