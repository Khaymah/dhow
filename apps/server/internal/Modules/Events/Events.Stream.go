package events

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	policy "github.com/Khaymah/dhow/apps/server/internal/Core/Policy"
	streams "github.com/Khaymah/dhow/apps/server/internal/Core/Streams"
	models "github.com/Khaymah/dhow/apps/server/internal/Modules/Events/Models"
)

type Access interface {
	Can(ctx context.Context, action policy.Action, project uuid.UUID) (bool, error)
}

type AccessFunc func(ctx context.Context, org uuid.UUID) (Access, error)

type OpenRequest struct {
	Organization uuid.UUID
	User         uuid.UUID
	Last         *Position
	Access       AccessFunc
}

type Stream struct {
	service *Service
	ctx     context.Context
	req     OpenRequest
	cursor  Position
	reset   bool
	release func()
	access  Access
	checked time.Time
}

func ParseLastEventID(raw string) (*Position, error) {
	if raw == "" {
		return nil, nil
	}
	epoch, seq, ok := strings.Cut(raw, "-")
	if !ok {
		return nil, apperrors.ValidationFailed(invalid("Last-Event-ID"))
	}
	e, err := strconv.ParseInt(epoch, 10, 64)
	if err != nil || e < 0 {
		return nil, apperrors.ValidationFailed(invalid("Last-Event-ID"))
	}
	q, err := strconv.ParseUint(seq, 10, 64)
	if err != nil {
		return nil, apperrors.ValidationFailed(invalid("Last-Event-ID"))
	}
	return &Position{Epoch: e, Seq: q}, nil
}

func (p Position) ID() string {
	return strconv.FormatInt(p.Epoch, 10) + "-" + strconv.FormatUint(p.Seq, 10)
}

func invalid(field string) apperrors.IErrorDetail {
	return apperrors.IErrorDetail{Field: field, Message: messageInvalidValue}
}

func (s *Service) Open(ctx context.Context, req OpenRequest) (*Stream, error) {
	if !s.hub.Healthy() {
		return nil, apperrors.ServiceUnavailable()
	}
	if err := s.openRate.Allow(req.User, s.now()); err != nil {
		return nil, err
	}
	release, err := s.limiter.Acquire(req.User, req.Organization)
	if err != nil {
		return nil, err
	}
	cursor := s.hub.Now()
	st := &Stream{service: s, ctx: ctx, req: req, cursor: cursor, release: release}
	if req.Last != nil {
		if s.hub.Resumable(*req.Last) {
			st.cursor = *req.Last
		} else {
			st.reset = true
		}
	}
	return st, nil
}

func (st *Stream) Serve(w http.ResponseWriter) error {
	defer st.release()
	s := st.service
	wake, unsubscribe := s.hub.Subscribe(st.req.Organization)
	defer unsubscribe()
	out := streams.NewWriter(w, s.wait, config.OrgEventsRetry)
	if err := out.Start(); err != nil {
		return nil
	}
	tick := time.NewTicker(s.tick)
	defer tick.Stop()
	if st.reset {
		if err := st.sendReset(out); err != nil {
			return nil
		}
	}
	for {
		if err := st.drain(out); err != nil {
			st.closed(err)
			return nil
		}
		select {
		case <-st.ctx.Done():
			return nil
		case <-s.stop:
			return nil
		case <-wake:
		case <-tick.C:
			if _, err := st.freshAccess(); err != nil {
				return nil
			}
			if err := out.Comment(); err != nil {
				st.closed(err)
				return nil
			}
		}
	}
}

var errEnded = errors.New("the live stream ended")

func (st *Stream) sendReset(out *streams.Writer) error {
	now := st.service.hub.Now()
	st.cursor = now
	if err := out.Event(models.NameReset, now.ID(), models.Reset{Epoch: now.Epoch}); err != nil {
		return err
	}
	return out.Flush()
}

func (st *Stream) drain(out *streams.Writer) error {
	s := st.service
	for {
		if !s.hub.Healthy() {
			return errEnded
		}
		b := s.hub.Since(st.req.Organization, st.cursor, config.OrgEventsBatch)
		if b.Lost {
			if err := st.sendReset(out); err != nil {
				return err
			}
			continue
		}
		if len(b.Entries) > 0 {
			if err := st.send(out, b.Entries); err != nil {
				return err
			}
		}
		st.cursor = b.Cursor
		if !b.More {
			return out.Flush()
		}
	}
}

func (st *Stream) currentAccess() (Access, error) {
	now := time.Now()
	if st.access != nil && st.service.ttl > 0 && now.Sub(st.checked) < st.service.ttl {
		return st.access, nil
	}
	return st.freshAccess()
}

func (st *Stream) freshAccess() (Access, error) {
	access, err := st.req.Access(st.ctx, st.req.Organization)
	if err != nil {
		st.access = nil
		return nil, err
	}
	st.access, st.checked = access, time.Now()
	return access, nil
}

func (st *Stream) send(out *streams.Writer, entries []entry) error {
	access, err := st.currentAccess()
	if err != nil {
		return errEnded
	}
	for _, e := range entries {
		if e.user != uuid.Nil && e.user != st.req.User {
			continue
		}
		project := uuid.Nil
		if e.event.ProjectID != nil {
			project = *e.event.ProjectID
		}
		ok, err := access.Can(st.ctx, e.action, project)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		pos := Position{Epoch: st.cursor.Epoch, Seq: e.event.Seq}
		if err := out.Event(models.NameChange, pos.ID(), e.event); err != nil {
			return err
		}
	}
	return nil
}

func (st *Stream) closed(err error) {
	if errors.Is(err, errEnded) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	st.service.log.InfoContext(context.WithoutCancel(st.ctx), "a live event stream was closed",
		"organization_id", st.req.Organization.String(), "slow_client", errors.Is(err, os.ErrDeadlineExceeded), "error", err)
}
