# 04 — Auth System

## Overview

The CMS supports two authentication modes: local (bcrypt password + JWT) and SSO via Logto (OIDC). Both modes issue the same JWT format with the same claims structure. Refresh tokens are stored in the `sessions` table. This guide walks through wiring the user repository, implementing the full login handler, issuing JWTs, handling refresh, and integrating the Logto SSO callback.

## Prerequisites

- Guide 03 completed (migrations applied, `users` and `sessions` tables exist).
- `.env` has `JWT_SECRET`, `JWT_EXPIRY`, and `REFRESH_TOKEN_EXPIRY` set.
- For SSO: `ENABLE_SSO=true`, `LOGTO_ENDPOINT`, `LOGTO_APP_ID`, `LOGTO_APP_SECRET`, `LOGTO_REDIRECT_URI` set.

## Step-by-Step

### 1. JWT claims structure

The `Claims` struct is defined in `backend/internal/adapters/http/middleware/auth.go`:

```go
// Claims represents the JWT payload.
type Claims struct {
    jwt.RegisteredClaims              // sub (user UUID), exp, iat
    TenantID string   `json:"tid"`   // tenant UUID as string
    Email    string   `json:"email"`
    Roles    []string `json:"roles"` // role names, e.g. ["Administrator"]
}
```

Decoded token example:

```json
{
  "sub": "550e8400-e29b-41d4-a716-446655440000",
  "exp": 1735689600,
  "iat": 1735603200,
  "tid": "00000000-0000-0000-0000-000000000001",
  "email": "admin@example.com",
  "roles": ["Administrator"]
}
```

Super admins additionally have `"super_admin"` in the roles array. The `SuperAdminGate` middleware checks for this string — it never stores the whitelist in the JWT.

### 2. Implement the user repository (PostgreSQL adapter)

Create `backend/internal/adapters/postgres/user_repository.go`:

```go
package postgres

import (
    "context"
    "fmt"
    "time"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5"
    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/imaginacolombia/cms/internal/core/domain/user"
    "github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

type UserRepository struct {
    db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
    return &UserRepository{db: db}
}

func (r *UserRepository) Create(ctx context.Context, u *user.User) error {
    const q = `
        INSERT INTO users (
            id, tenant_id, email, first_name, last_name, avatar,
            status, provider, provider_uid, password_hash, language,
            created_at, updated_at
        ) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`

    _, err := r.db.Exec(ctx, q,
        u.ID, u.TenantID, u.Email, u.FirstName, u.LastName, u.Avatar,
        u.Status, u.Provider, u.ProviderUID, u.PasswordHash, u.Language,
        u.CreatedAt, u.UpdatedAt,
    )
    return err
}

func (r *UserRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (*user.User, error) {
    const q = `
        SELECT id, tenant_id, email, first_name, last_name, avatar,
               status, provider, provider_uid, password_hash, last_login, language,
               created_at, updated_at
        FROM   users
        WHERE  tenant_id = $1 AND id = $2`

    return scanUser(r.db.QueryRow(ctx, q, tenantID, id))
}

func (r *UserRepository) GetByEmail(ctx context.Context, tenantID uuid.UUID, email string) (*user.User, error) {
    const q = `
        SELECT id, tenant_id, email, first_name, last_name, avatar,
               status, provider, provider_uid, password_hash, last_login, language,
               created_at, updated_at
        FROM   users
        WHERE  tenant_id = $1 AND email = $2`

    return scanUser(r.db.QueryRow(ctx, q, tenantID, email))
}

func (r *UserRepository) GetByProviderUID(
    ctx context.Context,
    tenantID uuid.UUID,
    provider user.AuthProvider,
    uid string,
) (*user.User, error) {
    const q = `
        SELECT id, tenant_id, email, first_name, last_name, avatar,
               status, provider, provider_uid, password_hash, last_login, language,
               created_at, updated_at
        FROM   users
        WHERE  tenant_id = $1 AND provider = $2 AND provider_uid = $3`

    return scanUser(r.db.QueryRow(ctx, q, tenantID, provider, uid))
}

func (r *UserRepository) SetPassword(ctx context.Context, userID uuid.UUID, hash string) error {
    _, err := r.db.Exec(ctx,
        `UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2`,
        hash, userID,
    )
    return err
}

func (r *UserRepository) RecordLogin(ctx context.Context, userID uuid.UUID) error {
    _, err := r.db.Exec(ctx,
        `UPDATE users SET last_login = NOW() WHERE id = $1`,
        userID,
    )
    return err
}

func (r *UserRepository) Update(ctx context.Context, u *user.User) error {
    _, err := r.db.Exec(ctx, `
        UPDATE users
        SET    first_name = $1, last_name = $2, avatar = $3,
               status = $4, language = $5, updated_at = NOW()
        WHERE  tenant_id = $6 AND id = $7`,
        u.FirstName, u.LastName, u.Avatar,
        u.Status, u.Language,
        u.TenantID, u.ID,
    )
    return err
}

func (r *UserRepository) Delete(ctx context.Context, tenantID, id uuid.UUID) error {
    _, err := r.db.Exec(ctx,
        `DELETE FROM users WHERE tenant_id = $1 AND id = $2`,
        tenantID, id,
    )
    return err
}

func (r *UserRepository) List(
    ctx context.Context,
    tenantID uuid.UUID,
    filter outbound.UserFilter,
) ([]*user.User, int64, error) {
    args := []any{tenantID}
    where := "tenant_id = $1"
    i := 2

    if filter.Status != nil {
        where += fmt.Sprintf(" AND status = $%d", i)
        args = append(args, *filter.Status)
        i++
    }
    if filter.Search != "" {
        where += fmt.Sprintf(` AND (email ILIKE $%d OR first_name ILIKE $%d OR last_name ILIKE $%d)`, i, i, i)
        args = append(args, "%"+filter.Search+"%")
        i++
    }

    var total int64
    _ = r.db.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE "+where, args...).Scan(&total)

    limit := filter.Limit
    if limit <= 0 {
        limit = 25
    }
    offset := (filter.Page - 1) * limit
    if offset < 0 {
        offset = 0
    }

    args = append(args, limit, offset)
    rows, err := r.db.Query(ctx, fmt.Sprintf(`
        SELECT id, tenant_id, email, first_name, last_name, avatar,
               status, provider, provider_uid, password_hash, last_login, language,
               created_at, updated_at
        FROM   users WHERE %s
        ORDER  BY created_at DESC
        LIMIT  $%d OFFSET $%d`, where, i, i+1), args...)
    if err != nil {
        return nil, 0, err
    }
    defer rows.Close()

    var users []*user.User
    for rows.Next() {
        u, err := scanUser(rows)
        if err != nil {
            return nil, 0, err
        }
        users = append(users, u)
    }
    return users, total, rows.Err()
}

func scanUser(row pgx.Row) (*user.User, error) {
    u := &user.User{}
    err := row.Scan(
        &u.ID, &u.TenantID, &u.Email, &u.FirstName, &u.LastName, &u.Avatar,
        &u.Status, &u.Provider, &u.ProviderUID, &u.PasswordHash, &u.LastLogin, &u.Language,
        &u.CreatedAt, &u.UpdatedAt,
    )
    if err != nil {
        return nil, fmt.Errorf("scanUser: %w", err)
    }
    return u, nil
}
```

### 3. Implement the session repository

Create `backend/internal/adapters/postgres/session_repository.go`:

```go
package postgres

import (
    "context"
    "fmt"
    "time"

    "github.com/google/uuid"
    "github.com/jackc/pgx/v5/pgxpool"

    "github.com/imaginacolombia/cms/internal/core/ports/outbound"
)

type SessionRepository struct {
    db *pgxpool.Pool
}

func NewSessionRepository(db *pgxpool.Pool) *SessionRepository {
    return &SessionRepository{db: db}
}

func (r *SessionRepository) Create(ctx context.Context, s *outbound.Session) error {
    const q = `
        INSERT INTO sessions (id, tenant_id, user_id, refresh_token, expires_at, ip_address, user_agent)
        VALUES ($1, $2, $3, $4, $5, $6, $7)`
    _, err := r.db.Exec(ctx, q,
        s.ID, s.TenantID, s.UserID, s.RefreshToken, s.ExpiresAt, s.IPAddress, s.UserAgent,
    )
    return err
}

func (r *SessionRepository) GetByToken(ctx context.Context, token string) (*outbound.Session, error) {
    const q = `
        SELECT id, tenant_id, user_id, refresh_token, expires_at
        FROM   sessions
        WHERE  refresh_token = $1 AND expires_at > NOW()`
    s := &outbound.Session{}
    err := r.db.QueryRow(ctx, q, token).Scan(
        &s.ID, &s.TenantID, &s.UserID, &s.RefreshToken, &s.ExpiresAt,
    )
    if err != nil {
        return nil, fmt.Errorf("session not found: %w", err)
    }
    return s, nil
}

func (r *SessionRepository) Delete(ctx context.Context, token string) error {
    _, err := r.db.Exec(ctx, `DELETE FROM sessions WHERE refresh_token = $1`, token)
    return err
}

func (r *SessionRepository) DeleteAllForUser(ctx context.Context, tenantID, userID uuid.UUID) error {
    _, err := r.db.Exec(ctx,
        `DELETE FROM sessions WHERE tenant_id = $1 AND user_id = $2`,
        tenantID, userID,
    )
    return err
}
```

### 4. Complete the login handler

Update `backend/internal/adapters/http/rest/auth.go` to wire the real repositories:

```go
func (h *authHandler) login(w http.ResponseWriter, r *http.Request) {
    var req loginRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        writeError(w, http.StatusBadRequest, "invalid request body")
        return
    }

    tenantID, ok := httpMiddleware.TenantIDFromCtx(r.Context())
    if !ok {
        writeError(w, http.StatusBadRequest, "tenant not resolved")
        return
    }

    // 1. Fetch user by email within the tenant
    u, err := h.userRepo.GetByEmail(r.Context(), tenantID, req.Email)
    if err != nil {
        // Use constant-time comparison to prevent user enumeration
        bcrypt.CompareHashAndPassword([]byte("$2a$10$placeholder"), []byte(req.Password)) //nolint
        writeError(w, http.StatusUnauthorized, "invalid credentials")
        return
    }

    // 2. Reject inactive accounts before password check
    if !u.IsActive() {
        writeError(w, http.StatusForbidden, "account is "+string(u.Status))
        return
    }

    // 3. Verify password
    if u.PasswordHash == nil {
        writeError(w, http.StatusUnauthorized, "password login not enabled for this account")
        return
    }
    if err := bcrypt.CompareHashAndPassword([]byte(*u.PasswordHash), []byte(req.Password)); err != nil {
        writeError(w, http.StatusUnauthorized, "invalid credentials")
        return
    }

    // 4. Collect role names for JWT
    roleNames := make([]string, len(u.Roles))
    for i, role := range u.Roles {
        roleNames[i] = role.Name
    }

    // 5. Issue access token
    expiresAt := time.Now().Add(h.cfg.Auth.JWTExpiry)
    accessToken, err := h.issueToken(tenantID, u.ID, u.Email, roleNames, expiresAt)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "could not issue token")
        return
    }

    // 6. Issue refresh token and persist session
    refreshToken := uuid.New().String()
    refreshExpiry := time.Now().Add(h.cfg.Auth.RefreshTokenExpiry)
    ip := r.RemoteAddr
    ua := r.UserAgent()
    _ = h.sessionRepo.Create(r.Context(), &outbound.Session{
        ID:           uuid.New(),
        TenantID:     tenantID,
        UserID:       u.ID,
        RefreshToken: refreshToken,
        ExpiresAt:    refreshExpiry,
        IPAddress:    &ip,
        UserAgent:    &ua,
    })

    // 7. Record last login (non-blocking)
    go h.userRepo.RecordLogin(context.Background(), u.ID)

    // 8. Publish event
    _ = h.bus.Publish(r.Context(), events.Event{
        Type:      events.EventUserLogin,
        TenantID:  tenantID,
        UserID:    &u.ID,
        UserEmail: u.Email,
    })

    writeJSON(w, http.StatusOK, tokenResponse{
        AccessToken:  accessToken,
        RefreshToken: refreshToken,
        ExpiresAt:    expiresAt,
        User: userInfo{
            ID:        u.ID.String(),
            Email:     u.Email,
            FirstName: u.FirstName,
            LastName:  u.LastName,
        },
    })
}
```

### 5. Implement token refresh

```go
func (h *authHandler) refresh(w http.ResponseWriter, r *http.Request) {
    var body struct {
        RefreshToken string `json:"refresh_token"`
    }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        writeError(w, http.StatusBadRequest, "invalid body")
        return
    }

    // Validate and fetch session
    session, err := h.sessionRepo.GetByToken(r.Context(), body.RefreshToken)
    if err != nil {
        writeError(w, http.StatusUnauthorized, "refresh token invalid or expired")
        return
    }

    // Fetch user to get current roles
    u, err := h.userRepo.GetByID(r.Context(), session.TenantID, session.UserID)
    if err != nil || !u.IsActive() {
        writeError(w, http.StatusUnauthorized, "user not found or inactive")
        return
    }

    roleNames := make([]string, len(u.Roles))
    for i, role := range u.Roles {
        roleNames[i] = role.Name
    }

    // Issue new access token (rotate — do NOT issue new refresh token unless implementing refresh token rotation)
    expiresAt := time.Now().Add(h.cfg.Auth.JWTExpiry)
    accessToken, err := h.issueToken(session.TenantID, u.ID, u.Email, roleNames, expiresAt)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "could not issue token")
        return
    }

    _ = h.bus.Publish(r.Context(), events.Event{
        Type:     events.EventTokenRefreshed,
        TenantID: session.TenantID,
        UserID:   &u.ID,
    })

    writeJSON(w, http.StatusOK, map[string]any{
        "access_token": accessToken,
        "expires_at":   expiresAt,
    })
}
```

### 6. Implement user registration (for auth collections)

```go
// POST /api/auth/register
func (h *authHandler) register(w http.ResponseWriter, r *http.Request) {
    tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())

    var body struct {
        Email     string `json:"email"`
        Password  string `json:"password"`
        FirstName string `json:"first_name"`
        LastName  string `json:"last_name"`
    }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        writeError(w, http.StatusBadRequest, "invalid body")
        return
    }

    // Hash password — cost 12 is the recommended minimum
    hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), 12)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "could not hash password")
        return
    }
    hashStr := string(hash)

    u := user.New(tenantID, body.Email, body.FirstName, body.LastName, user.ProviderLocal)
    u.PasswordHash = &hashStr

    if err := h.userRepo.Create(r.Context(), u); err != nil {
        writeError(w, http.StatusConflict, "email already registered")
        return
    }

    writeJSON(w, http.StatusCreated, map[string]any{"id": u.ID})
}
```

### 7. Implement the Logto SSO callback

When `ENABLE_SSO=true`, the router mounts `/api/auth/sso` and `/api/auth/sso/callback`.

```go
func (h *authHandler) ssoRedirect(w http.ResponseWriter, r *http.Request) {
    // Build the Logto OIDC authorization URL
    state := uuid.New().String()
    params := url.Values{
        "client_id":     {h.cfg.Logto.AppID},
        "redirect_uri":  {h.cfg.Logto.RedirectURI},
        "response_type": {"code"},
        "scope":         {"openid profile email"},
        "state":         {state},
    }

    // Store state in a short-lived cookie for CSRF validation
    http.SetCookie(w, &http.Cookie{
        Name:     "sso_state",
        Value:    state,
        MaxAge:   300,
        HttpOnly: true,
        Path:     "/",
    })

    http.Redirect(w, r,
        h.cfg.Logto.Endpoint+"/oidc/auth?"+params.Encode(),
        http.StatusFound,
    )
}

func (h *authHandler) ssoCallback(w http.ResponseWriter, r *http.Request) {
    // 1. Validate state
    stateCookie, err := r.Cookie("sso_state")
    if err != nil || stateCookie.Value != r.URL.Query().Get("state") {
        writeError(w, http.StatusBadRequest, "invalid state")
        return
    }

    // 2. Exchange authorization code for tokens
    code := r.URL.Query().Get("code")
    tokenResp, err := exchangeLogtoCode(r.Context(), h.cfg.Logto, code)
    if err != nil {
        h.log.Errorw("logto token exchange", "err", err)
        writeError(w, http.StatusInternalServerError, "SSO token exchange failed")
        return
    }

    // 3. Fetch user info from Logto userinfo endpoint
    info, err := fetchLogtoUserInfo(r.Context(), h.cfg.Logto.Endpoint, tokenResp.AccessToken)
    if err != nil {
        writeError(w, http.StatusInternalServerError, "could not fetch user info")
        return
    }

    tenantID, _ := httpMiddleware.TenantIDFromCtx(r.Context())

    // 4. Upsert user (create if not exists, update if returning)
    u, err := h.userRepo.GetByProviderUID(r.Context(), tenantID, user.ProviderSSO, info.Sub)
    if err != nil {
        // First SSO login — create user
        u = user.New(tenantID, info.Email, info.GivenName, info.FamilyName, user.ProviderSSO)
        uid := info.Sub
        u.ProviderUID = &uid
        if createErr := h.userRepo.Create(r.Context(), u); createErr != nil {
            writeError(w, http.StatusInternalServerError, "could not create user")
            return
        }
    }

    if !u.IsActive() {
        writeError(w, http.StatusForbidden, "account suspended")
        return
    }

    // 5. Issue CMS JWT and redirect to frontend
    expiresAt := time.Now().Add(h.cfg.Auth.JWTExpiry)
    token, _ := h.issueToken(tenantID, u.ID, u.Email, nil, expiresAt)

    // Set token as cookie and redirect to the admin SPA
    http.SetCookie(w, &http.Cookie{
        Name:     "access_token",
        Value:    token,
        Expires:  expiresAt,
        HttpOnly: true,
        Path:     "/",
        SameSite: http.SameSiteLaxMode,
    })
    http.Redirect(w, r, "/", http.StatusFound)
}

// exchangeLogtoCode POSTs to Logto's token endpoint.
func exchangeLogtoCode(ctx context.Context, cfg config.LogtoConfig, code string) (*logtoTokenResponse, error) {
    vals := url.Values{
        "grant_type":   {"authorization_code"},
        "code":         {code},
        "redirect_uri": {cfg.RedirectURI},
    }
    req, _ := http.NewRequestWithContext(ctx,
        http.MethodPost,
        cfg.Endpoint+"/oidc/token",
        strings.NewReader(vals.Encode()),
    )
    req.SetBasicAuth(cfg.AppID, cfg.AppSecret)
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var t logtoTokenResponse
    return &t, json.NewDecoder(resp.Body).Decode(&t)
}

type logtoTokenResponse struct {
    AccessToken string `json:"access_token"`
    IDToken     string `json:"id_token"`
}

type logtoUserInfo struct {
    Sub        string `json:"sub"`
    Email      string `json:"email"`
    GivenName  string `json:"given_name"`
    FamilyName string `json:"family_name"`
}

func fetchLogtoUserInfo(ctx context.Context, endpoint, accessToken string) (*logtoUserInfo, error) {
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"/oidc/me", nil)
    req.Header.Set("Authorization", "Bearer "+accessToken)
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    var info logtoUserInfo
    return &info, json.NewDecoder(resp.Body).Decode(&info)
}
```

### 8. Wire repositories into the auth handler

Update `MountAuth` in `rest/auth.go`:

```go
func MountAuth(r chi.Router, cfg *config.Config, db *pgxpool.Pool, bus events.Bus, log *logger.Logger) {
    h := &authHandler{
        cfg:         cfg,
        bus:         bus,
        log:         log,
        userRepo:    postgresAdapter.NewUserRepository(db),
        sessionRepo: postgresAdapter.NewSessionRepository(db),
    }
    // ... routes unchanged
}
```

And update `authHandler` struct:

```go
type authHandler struct {
    cfg         *config.Config
    bus         events.Bus
    log         *logger.Logger
    userRepo    outbound.UserRepository
    sessionRepo outbound.SessionRepository
}
```

### 9. Test the login flow

```bash
# Login
curl -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -H "X-Tenant-ID: 00000000-0000-0000-0000-000000000001" \
  -d '{"email":"admin@example.com","password":"secret"}'

# Response:
# {
#   "access_token": "eyJ...",
#   "refresh_token": "550e8400...",
#   "expires_at": "2024-...",
#   "user": {"id": "...", "email": "admin@example.com"}
# }

# Refresh
curl -X POST http://localhost:8080/api/auth/refresh \
  -H "Content-Type: application/json" \
  -d '{"refresh_token":"550e8400..."}'
```

## Key Files

- `/home/jd/dev/full/cms/backend/internal/adapters/http/middleware/auth.go` — JWT Claims struct + extraction
- `/home/jd/dev/full/cms/backend/internal/adapters/http/middleware/superadmin.go` — super admin gate
- `/home/jd/dev/full/cms/backend/internal/adapters/http/rest/auth.go` — login/logout/refresh/SSO handlers
- `/home/jd/dev/full/cms/backend/internal/core/domain/user/entity.go` — User, Role, Permission types
- `/home/jd/dev/full/cms/backend/internal/core/ports/outbound/repositories.go` — UserRepository + SessionRepository interfaces
- `/home/jd/dev/full/cms/backend/internal/infrastructure/database/migrations/001_initial_schema.sql` — users + sessions tables

## Next Steps

Proceed to [05 — Multi-Tenancy](./05-multi-tenancy.md) to understand tenant isolation, the resolver middleware, super admin cross-tenant access, and white-label configuration.
