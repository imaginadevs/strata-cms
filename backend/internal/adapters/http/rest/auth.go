package rest

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	httpMiddleware "github.com/imaginacolombia/cms/internal/adapters/http/middleware"
	postgresrepo "github.com/imaginacolombia/cms/internal/adapters/persistence/postgres"
	auditdomain "github.com/imaginacolombia/cms/internal/core/domain/audit"
	collectiondomain "github.com/imaginacolombia/cms/internal/core/domain/collection"
	"github.com/imaginacolombia/cms/internal/core/domain/ids"
	"github.com/imaginacolombia/cms/internal/core/events"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
	"github.com/imaginacolombia/cms/internal/infrastructure/config"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

// MountAuth registers auth routes on the provided (public) router group.
func MountAuth(r chi.Router, cfg *config.Config, repos *postgresrepo.Repos, bus events.Bus, log *logger.Logger) {
	h := &authHandler{cfg: cfg, repos: repos, bus: bus, log: log}

	r.Post("/auth/login", h.login)
	r.Post("/auth/logout", h.logout)
	r.Post("/auth/refresh", h.refresh)
	r.Post("/auth/accept-invite", h.acceptInvite)
	// /auth/me needs the JWT parsed (from the access_token cookie or header) to
	// identify the caller — the public group has no Authenticate middleware, so
	// attach it just for this route. This is what restores the session on reload.
	r.With(httpMiddleware.Authenticate(cfg.Auth.JWTSecret)).Get("/auth/me", h.me)

	// Auth-type collections (e.g. ecommerce_customers, cmms_operators) each
	// get their own login endpoint, scoped by collection name. These issue
	// "auth" principal tokens instead of system cms_users tokens.
	r.Post("/auth/{collection}/login", h.collectionLogin)

	if cfg.Features.SSO {
		r.Get("/auth/sso", h.ssoRedirect)
		r.Get("/auth/sso/callback", h.ssoCallback)
	}
}

type authHandler struct {
	cfg   *config.Config
	repos *postgresrepo.Repos
	bus   events.Bus
	log   *logger.Logger
}

// ─── login ────────────────────────────────────────────────────────────────────

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// tokenResponse is returned by login/refresh. Clients hold the JWT and send it
// as `Authorization: Bearer <access_token>` (the middleware also accepts it via
// an access_token cookie). This uniform token model serves the admin SPA,
// external/API consumers, and cross-tenant tooling alike.
type tokenResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
	User         userInfo  `json:"user"`
}

type userInfo struct {
	ID           string   `json:"id"`
	Email        string   `json:"email"`
	FirstName    string   `json:"first_name"`
	LastName     string   `json:"last_name"`
	Avatar       *string  `json:"avatar,omitempty"`
	Roles        []string `json:"roles"`
	IsSuperAdmin bool     `json:"is_super_admin"`
	Language     string   `json:"language"`
	UILanguage   string   `json:"ui_language"`
}

func (h *authHandler) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "email and password are required")
		return
	}

	tenantID, ok := httpMiddleware.TenantIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "tenant not resolved")
		return
	}

	u, err := h.repos.User.GetByEmail(r.Context(), tenantID, req.Email)
	if err != nil {
		h.log.Errorw("user lookup failed", "err", err)
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}
	if u == nil || u.PasswordHash == nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*u.PasswordHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if !u.IsActive() {
		writeError(w, http.StatusForbidden, "account is inactive")
		return
	}

	roles := make([]string, 0, len(u.Roles))
	for _, role := range u.Roles {
		roles = append(roles, role.Name)
	}

	accessExpiry := time.Now().Add(h.cfg.Auth.JWTExpiry)
	accessToken, err := h.issueToken(tenantID, u.ID, u.Email, roles, u.IsSuperAdmin, httpMiddleware.PrincipalUser, "", accessExpiry)
	if err != nil {
		h.log.Errorw("token issue failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}

	refreshToken, refreshExpiry, err := h.newRefreshToken()
	if err != nil {
		h.log.Errorw("refresh token generation failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	ipAddr, ua := requestOrigin(r)
	sess := &outbound.Session{
		ID:           ids.New(),
		UserID:       &u.ID,
		RefreshToken: refreshToken,
		ExpiresAt:    refreshExpiry,
		IPAddress:    &ipAddr,
		UserAgent:    &ua,
	}
	if err := h.repos.Session.Create(r.Context(), tenantID, sess); err != nil {
		h.log.Errorw("session create failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}

	_ = h.repos.User.RecordLogin(r.Context(), tenantID, u.ID)

	recordAudit(r.Context(), h.repos, h.log, r, tenantID, &u.ID, &u.Email,
		"auth", nil, auditdomain.OperationLogin, nil, nil)

	_ = h.bus.Publish(r.Context(), events.Event{
		Type:      events.EventUserLogin,
		TenantID:  tenantID,
		UserID:    &u.ID,
		UserEmail: u.Email,
	})

	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    accessExpiry,
		User: userInfo{
			ID:           u.ID.String(),
			Email:        u.Email,
			FirstName:    u.FirstName,
			LastName:     u.LastName,
			Avatar:       u.Avatar,
			Roles:        roles,
			IsSuperAdmin: u.IsSuperAdmin,
			Language:     u.Language,
			UILanguage:   u.UILanguage,
		},
	})
}

// ─── accept invite ─────────────────────────────────────────────────────────────

// acceptInvite activates an invited user: it validates the invite token, sets the
// chosen password, marks the account active, and logs the user in (same token
// response as login). Public route; tenant resolved from the Host domain.
func (h *authHandler) acceptInvite(w http.ResponseWriter, r *http.Request) {
	tenantID, ok := httpMiddleware.TenantIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "tenant not resolved")
		return
	}
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Token == "" || len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "token and a password of at least 8 characters are required")
		return
	}

	u, err := h.repos.User.GetByInviteToken(r.Context(), tenantID, hashToken(req.Token))
	if err != nil {
		h.log.Errorw("accept invite: lookup failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not accept invite")
		return
	}
	if u == nil {
		writeError(w, http.StatusBadRequest, "this invitation is invalid or has expired")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not set password")
		return
	}
	if err := h.repos.User.AcceptInvite(r.Context(), tenantID, u.ID, string(hash)); err != nil {
		h.log.Errorw("accept invite: activation failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not accept invite")
		return
	}

	// Reload with roles so the issued token + response are complete.
	if full, err := h.repos.User.GetByID(r.Context(), tenantID, u.ID); err == nil && full != nil {
		u = full
	}
	roles := make([]string, 0, len(u.Roles))
	for _, role := range u.Roles {
		roles = append(roles, role.Name)
	}

	accessExpiry := time.Now().Add(h.cfg.Auth.JWTExpiry)
	accessToken, err := h.issueToken(tenantID, u.ID, u.Email, roles, u.IsSuperAdmin, httpMiddleware.PrincipalUser, "", accessExpiry)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}
	refreshToken, refreshExpiry, err := h.newRefreshToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	ipAddr, ua := requestOrigin(r)
	sess := &outbound.Session{
		ID:           ids.New(),
		UserID:       &u.ID,
		RefreshToken: refreshToken,
		ExpiresAt:    refreshExpiry,
		IPAddress:    &ipAddr,
		UserAgent:    &ua,
	}
	if err := h.repos.Session.Create(r.Context(), tenantID, sess); err != nil {
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	_ = h.repos.User.RecordLogin(r.Context(), tenantID, u.ID)

	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    accessExpiry,
		User: userInfo{
			ID:           u.ID.String(),
			Email:        u.Email,
			FirstName:    u.FirstName,
			LastName:     u.LastName,
			Avatar:       u.Avatar,
			Roles:        roles,
			IsSuperAdmin: u.IsSuperAdmin,
			Language:     u.Language,
			UILanguage:   u.UILanguage,
		},
	})
}

// ─── collection login (auth-type collections) ─────────────────────────────────

type collectionLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// collectionLogin authenticates against a row in a custom auth-type collection
// (e.g. ecommerce_customers, cmms_operators) instead of cms_users. The role
// granted is fixed per-collection via collections.meta.auth_default_role,
// configured when the collection/plugin is set up.
func (h *authHandler) collectionLogin(w http.ResponseWriter, r *http.Request) {
	coll := chi.URLParam(r, "collection")
	tenantID, ok := httpMiddleware.TenantIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "tenant not resolved")
		return
	}

	schema, err := h.repos.Collection.GetByName(r.Context(), tenantID, coll)
	if err != nil {
		h.log.Errorw("collection login: schema lookup failed", "err", err, "collection", coll)
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}
	if schema == nil || schema.Type != collectiondomain.CollectionTypeAuth {
		writeError(w, http.StatusNotFound, "not an auth collection")
		return
	}

	var req collectionLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	rows, _, err := h.repos.Item.List(r.Context(), tenantID, coll, outbound.QueryFilter{
		Filter: map[string]any{"username": req.Username},
		Limit:  1,
	})
	if err != nil {
		h.log.Errorw("collection login: lookup failed", "err", err, "collection", coll)
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}
	if len(rows) == 0 {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	item := rows[0]

	hash, _ := item["password_hash"].(string)
	if hash == "" {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	if status, _ := item["status"].(string); status != "" && status != "active" {
		writeError(w, http.StatusForbidden, "account is inactive")
		return
	}

	itemID, err := uuid.Parse(toIDString(item["id"]))
	if err != nil {
		h.log.Errorw("collection login: item id not a uuid", "collection", coll)
		writeError(w, http.StatusInternalServerError, "login failed")
		return
	}

	roleName, _ := schema.Meta["auth_default_role"].(string)
	var roles []string
	if roleName != "" {
		roles = []string{roleName}
	}

	accessExpiry := time.Now().Add(h.cfg.Auth.JWTExpiry)
	accessToken, err := h.issueToken(tenantID, itemID, req.Username, roles, false, httpMiddleware.PrincipalAuth, coll, accessExpiry)
	if err != nil {
		h.log.Errorw("collection login: token issue failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}

	refreshToken, refreshExpiry, err := h.newRefreshToken()
	if err != nil {
		h.log.Errorw("collection login: refresh token generation failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}
	ipAddr, ua := requestOrigin(r)
	sess := &outbound.Session{
		ID:                  ids.New(),
		PrincipalCollection: &coll,
		PrincipalID:         &itemID,
		RefreshToken:        refreshToken,
		ExpiresAt:           refreshExpiry,
		IPAddress:           &ipAddr,
		UserAgent:           &ua,
	}
	if err := h.repos.Session.Create(r.Context(), tenantID, sess); err != nil {
		h.log.Errorw("collection login: session create failed", "err", err)
		writeError(w, http.StatusInternalServerError, "could not create session")
		return
	}

	_ = h.bus.Publish(r.Context(), events.Event{
		Type:       events.EventUserLogin,
		TenantID:   tenantID,
		Collection: coll,
		ItemID:     strPtr(itemID.String()),
		UserEmail:  req.Username,
	})

	delete(item, "password_hash")
	writeJSON(w, http.StatusOK, map[string]any{
		"access_token":  accessToken,
		"refresh_token": refreshToken,
		"expires_at":    accessExpiry,
		"collection":    coll,
		"roles":         roles,
		"item":          item,
	})
}

// ─── logout ───────────────────────────────────────────────────────────────────

type logoutRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *authHandler) logout(w http.ResponseWriter, r *http.Request) {
	var req logoutRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	// Also accept refresh token from cookie
	if req.RefreshToken == "" {
		if c, err := r.Cookie("refresh_token"); err == nil {
			req.RefreshToken = c.Value
		}
	}

	userID, hasUser := httpMiddleware.UserIDFromCtx(r.Context())
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())
	email := httpMiddleware.EmailFromCtx(r.Context())

	if req.RefreshToken != "" {
		if err := h.repos.Session.Delete(r.Context(), tenantID, req.RefreshToken); err != nil {
			h.log.Warnw("session delete failed", "err", err)
		}
	}

	if hasUser {
		_ = h.bus.Publish(r.Context(), events.Event{
			Type:      events.EventUserLogout,
			TenantID:  tenantID,
			UserID:    &userID,
			UserEmail: email,
		})
	}

	http.SetCookie(w, &http.Cookie{Name: "access_token", MaxAge: -1, Path: "/"})
	http.SetCookie(w, &http.Cookie{Name: "refresh_token", MaxAge: -1, Path: "/"})
	w.WriteHeader(http.StatusNoContent)
}

// ─── refresh ──────────────────────────────────────────────────────────────────

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *authHandler) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.RefreshToken == "" {
		if c, err := r.Cookie("refresh_token"); err == nil {
			req.RefreshToken = c.Value
		}
	}
	if req.RefreshToken == "" {
		writeError(w, http.StatusBadRequest, "refresh_token is required")
		return
	}

	tenantID, ok := httpMiddleware.TenantIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "tenant not resolved")
		return
	}

	sess, err := h.repos.Session.GetByToken(r.Context(), tenantID, req.RefreshToken)
	if err != nil {
		h.log.Errorw("session lookup failed", "err", err)
		writeError(w, http.StatusInternalServerError, "refresh failed")
		return
	}
	if sess == nil {
		writeError(w, http.StatusUnauthorized, "invalid or expired refresh token")
		return
	}

	expiry, ok := sess.ExpiresAt.(time.Time)
	if !ok || expiry.Before(time.Now()) {
		_ = h.repos.Session.Delete(r.Context(), tenantID, req.RefreshToken)
		writeError(w, http.StatusUnauthorized, "refresh token expired")
		return
	}

	accessExpiry := time.Now().Add(h.cfg.Auth.JWTExpiry)

	// Auth-collection principal (customer/operator/etc — no cms_users row).
	if sess.PrincipalCollection != nil && sess.PrincipalID != nil {
		coll := *sess.PrincipalCollection
		item, err := h.repos.Item.GetByID(r.Context(), tenantID, coll, sess.PrincipalID.String(), outbound.QueryFilter{})
		if err != nil || item == nil {
			writeError(w, http.StatusUnauthorized, "principal not found")
			return
		}
		schema, err := h.repos.Collection.GetByName(r.Context(), tenantID, coll)
		if err != nil || schema == nil {
			writeError(w, http.StatusUnauthorized, "principal not found")
			return
		}
		roleName, _ := schema.Meta["auth_default_role"].(string)
		var roles []string
		if roleName != "" {
			roles = []string{roleName}
		}
		username, _ := item["username"].(string)
		accessToken, err := h.issueToken(tenantID, *sess.PrincipalID, username, roles, false, httpMiddleware.PrincipalAuth, coll, accessExpiry)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not issue token")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"access_token":  accessToken,
			"refresh_token": req.RefreshToken,
			"expires_at":    accessExpiry,
			"collection":    coll,
			"roles":         roles,
		})
		return
	}

	if sess.UserID == nil {
		writeError(w, http.StatusUnauthorized, "invalid session")
		return
	}
	u, err := h.repos.User.GetByID(r.Context(), tenantID, *sess.UserID)
	if err != nil || u == nil {
		writeError(w, http.StatusUnauthorized, "user not found")
		return
	}

	roles := make([]string, 0, len(u.Roles))
	for _, role := range u.Roles {
		roles = append(roles, role.Name)
	}

	accessToken, err := h.issueToken(tenantID, *sess.UserID, u.Email, roles, u.IsSuperAdmin, httpMiddleware.PrincipalUser, "", accessExpiry)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue token")
		return
	}

	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  accessToken,
		RefreshToken: req.RefreshToken,
		ExpiresAt:    accessExpiry,
		User: userInfo{
			ID:           u.ID.String(),
			Email:        u.Email,
			FirstName:    u.FirstName,
			LastName:     u.LastName,
			Avatar:       u.Avatar,
			Roles:        roles,
			IsSuperAdmin: u.IsSuperAdmin,
			Language:     u.Language,
			UILanguage:   u.UILanguage,
		},
	})
}

// ─── me ───────────────────────────────────────────────────────────────────────

func (h *authHandler) me(w http.ResponseWriter, r *http.Request) {
	userID, ok := httpMiddleware.UserIDFromCtx(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}
	tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())

	// Auth-collection principal — return the item row (minus password_hash).
	if coll, isAuth := httpMiddleware.AuthCollectionFromCtx(r.Context()); isAuth {
		item, err := h.repos.Item.GetByID(r.Context(), tenantID, coll, userID.String(), outbound.QueryFilter{})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not fetch item")
			return
		}
		if item == nil {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		delete(item, "password_hash")
		writeJSON(w, http.StatusOK, map[string]any{
			"collection": coll,
			"roles":      httpMiddleware.RolesFromCtx(r.Context()),
			"item":       item,
		})
		return
	}

	u, err := h.repos.User.GetByID(r.Context(), tenantID, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not fetch user")
		return
	}
	if u == nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}

	roles := make([]string, 0, len(u.Roles))
	for _, role := range u.Roles {
		roles = append(roles, role.Name)
	}

	writeJSON(w, http.StatusOK, userInfo{
		ID:           u.ID.String(),
		Email:        u.Email,
		FirstName:    u.FirstName,
		LastName:     u.LastName,
		Avatar:       u.Avatar,
		Roles:        roles,
		IsSuperAdmin: u.IsSuperAdmin,
		Language:     u.Language,
		UILanguage:   u.UILanguage,
	})
}

// ─── SSO stubs (Logto — implemented separately) ───────────────────────────────

func (h *authHandler) ssoRedirect(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "SSO not configured")
}

func (h *authHandler) ssoCallback(w http.ResponseWriter, r *http.Request) {
	writeError(w, http.StatusNotImplemented, "SSO not configured")
}

// ─── Token helpers ────────────────────────────────────────────────────────────

// issueToken signs a JWT for either a system user (principal="user", collection="")
// or an auth-collection principal (principal="auth", collection=<name>). subjectID
// is the cms_users.id or the auth-collection row id, respectively.
func (h *authHandler) issueToken(tenantID, subjectID uuid.UUID, email string, roles []string, isSuperAdmin bool, principal, collection string, expiresAt time.Time) (string, error) {
	claims := httpMiddleware.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subjectID.String(),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		TenantID:     tenantID.String(),
		Email:        email,
		Roles:        roles,
		IsSuperAdmin: isSuperAdmin,
		Principal:    principal,
		Collection:   collection,
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(h.cfg.Auth.JWTSecret))
}

// newRefreshToken generates a random opaque refresh token and its expiry.
func (h *authHandler) newRefreshToken() (string, time.Time, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	return hex.EncodeToString(raw), time.Now().Add(h.cfg.Auth.RefreshTokenExpiry), nil
}

// requestOrigin extracts a bare client IP and User-Agent for session records.
func requestOrigin(r *http.Request) (ip, userAgent string) {
	ip, _, _ = net.SplitHostPort(r.RemoteAddr)
	if ip == "" {
		ip = r.RemoteAddr
	}
	return ip, r.Header.Get("User-Agent")
}

func strPtr(s string) *string { return &s }
