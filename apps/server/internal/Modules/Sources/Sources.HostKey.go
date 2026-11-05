package sources

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	ratelimit "github.com/Khaymah/dhow/apps/server/internal/Core/RateLimit"
	"github.com/Khaymah/dhow/internal/gitsource"
	"github.com/Khaymah/dhow/internal/netguard"
	"github.com/Khaymah/dhow/internal/sshhostkey"
)

const (
	hostKeyUser   = "git"
	limitDialUser = "git-dial-user"
	limitDialOrg  = "git-dial-org"
)

func (s *Service) chargeDial(ctx context.Context, org, user uuid.UUID) error {
	if s.limiter == nil {
		return nil
	}
	for _, c := range []struct {
		limit   ratelimit.Limit
		subject string
	}{
		{ratelimit.Limit{Name: limitDialUser, Max: config.GitDialPerUser, Window: config.GitDialWindow}, user.String()},
		{ratelimit.Limit{Name: limitDialOrg, Max: config.GitDialPerOrg, Window: config.GitDialWindow}, org.String()},
	} {
		d, err := s.limiter.Take(ctx, c.limit, c.subject)
		if err != nil {
			return err
		}
		if err := d.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) fetchHostKey(ctx context.Context, u gitsource.URL) (ssh.PublicKey, error) {
	user := u.User
	if user == "" {
		user = hostKeyUser
	}
	key, err := sshhostkey.Fetch(ctx, s.dialer, u.HostPort(), user, config.HostKeyFetchTimeout)
	if errors.Is(err, netguard.ErrBlocked) {
		return nil, apperrors.GitHostBlocked()
	}
	if err != nil {
		return nil, apperrors.GitHostUnreachable(err)
	}
	return key, nil
}
