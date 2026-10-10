// Package apitokens contains the background job that auto-rotates monthly API
// tokens and emails admins ahead of each rotation.
package apitokens

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/imaginacolombia/cms/internal/core/domain/apitoken"
	"github.com/imaginacolombia/cms/internal/core/ports/outbound"
	"github.com/imaginacolombia/cms/internal/infrastructure/logger"
)

// Scheduler periodically rotates due monthly tokens (with a grace window) and
// sends admins a heads-up ~24h before each rotation. Email is best-effort and
// skipped entirely when no mailer is configured.
type Scheduler struct {
	tenants  outbound.TenantRepository
	tokens   outbound.APITokenRepository
	users    outbound.UserRepository
	mailer   outbound.Mailer // nil when SMTP is not configured
	log      *logger.Logger
	appName  string
	interval time.Duration
}

func NewScheduler(
	tenants outbound.TenantRepository,
	tokens outbound.APITokenRepository,
	users outbound.UserRepository,
	mailer outbound.Mailer,
	log *logger.Logger,
	appName string,
) *Scheduler {
	return &Scheduler{
		tenants:  tenants,
		tokens:   tokens,
		users:    users,
		mailer:   mailer,
		log:      log,
		appName:  appName,
		interval: time.Hour,
	}
}

// Start runs the loop until ctx is cancelled. Non-blocking.
func (s *Scheduler) Start(ctx context.Context) {
	go func() {
		t := time.NewTicker(s.interval)
		defer t.Stop()
		s.tick(ctx)
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.tick(ctx)
			}
		}
	}()
}

func (s *Scheduler) tick(ctx context.Context) {
	active := true
	tenants, _, err := s.tenants.List(ctx, outbound.TenantFilter{Active: &active, Limit: 10_000})
	if err != nil {
		s.log.Errorw("apitoken scheduler: list tenants failed", "err", err)
		return
	}
	now := time.Now().UTC()
	for _, tn := range tenants {
		toks, err := s.tokens.List(ctx, tn.ID)
		if err != nil {
			s.log.Warnw("apitoken scheduler: list tokens failed", "tenant", tn.ID, "err", err)
			continue
		}
		for _, tok := range toks {
			switch {
			case tok.DueForRotation(now):
				tok.Rotate(true, now) // grace rotation: old secret valid 24h more
				if err := s.tokens.Update(ctx, tn.ID, tok); err != nil {
					s.log.Errorw("apitoken scheduler: rotate update failed", "tenant", tn.ID, "token", tok.ID, "err", err)
					continue
				}
				s.notify(ctx, tn.ID, tn.Name, tok, true)
			case tok.DueForNotify(now):
				stamp := now
				tok.LastNotifiedAt = &stamp
				if err := s.tokens.Update(ctx, tn.ID, tok); err != nil {
					s.log.Errorw("apitoken scheduler: notify update failed", "tenant", tn.ID, "token", tok.ID, "err", err)
					continue
				}
				s.notify(ctx, tn.ID, tn.Name, tok, false)
			}
		}
	}
}

func (s *Scheduler) notify(ctx context.Context, tenantID uuid.UUID, tenantName string, tok *apitoken.APIToken, rotated bool) {
	if s.mailer == nil {
		return
	}
	emails, err := s.users.ListAdminEmails(ctx, tenantID)
	if err != nil || len(emails) == 0 {
		if err != nil {
			s.log.Warnw("apitoken scheduler: list admin emails failed", "err", err)
		}
		return
	}

	var subject, body string
	if rotated {
		subject = fmt.Sprintf("[%s] API token “%s” was rotated", s.appName, tok.Name)
		body = fmt.Sprintf(
			"The API token “%s” for %s has just been automatically rotated.\n\n"+
				"The previous secret keeps working for 24 hours. Open the admin → API Tokens screen "+
				"and use “Rotate” to reveal a fresh secret for your integrations before the grace window ends.",
			tok.Name, tenantName)
	} else {
		when := ""
		if tok.RotateAt != nil {
			when = tok.RotateAt.Format("2006-01-02 15:04 UTC")
		}
		subject = fmt.Sprintf("[%s] API token “%s” rotates in 24 hours", s.appName, tok.Name)
		body = fmt.Sprintf(
			"The API token “%s” for %s is scheduled to rotate at %s (in ~24 hours).\n\n"+
				"To avoid any interruption, open the admin → API Tokens screen and rotate it now to capture "+
				"the new secret and update your integrations. When it auto-rotates, the old secret stays valid for "+
				"a further 24 hours.",
			tok.Name, tenantName, when)
	}

	if err := s.mailer.Send(ctx, outbound.Message{To: emails, Subject: subject, Body: body}); err != nil {
		s.log.Warnw("apitoken scheduler: send email failed", "err", err, "recipients", strings.Join(emails, ","))
	}
}
