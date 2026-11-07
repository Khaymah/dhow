package events

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	auth "github.com/Khaymah/dhow/apps/server/internal/Core/Auth"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/Events/Gen"
)

const messageInvalidValue = "errors.validationFailed"

type Accessor interface {
	AccessIn(ctx context.Context, org uuid.UUID) (*auth.Access, error)
}

type Controller struct {
	service *Service
	guard   Accessor
}

var _ gen.StrictServerInterface = (*Controller)(nil)

func NewController(service *Service, guard Accessor) *Controller {
	return &Controller{service: service, guard: guard}
}

func (c *Controller) access(ctx context.Context, org uuid.UUID) (Access, error) {
	a, err := c.guard.AccessIn(ctx, org)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (c *Controller) StreamOrganizationEvents(ctx context.Context, req gen.StreamOrganizationEventsRequestObject) (gen.StreamOrganizationEventsResponseObject, error) {
	p, ok := auth.PrincipalFrom(ctx)
	if !ok {
		return nil, apperrors.Unauthenticated()
	}
	last := ""
	if req.Params.LastEventID != nil {
		last = *req.Params.LastEventID
	}
	pos, err := ParseLastEventID(last)
	if err != nil {
		return nil, err
	}
	stream, err := c.service.Open(ctx, OpenRequest{Organization: req.Id, User: p.UserID, Last: pos, Access: c.access})
	if err != nil {
		return nil, err
	}
	return streamResponse{stream: stream}, nil
}

type streamResponse struct {
	stream *Stream
}

func (r streamResponse) VisitStreamOrganizationEventsResponse(w http.ResponseWriter) error {
	return r.stream.Serve(w)
}
