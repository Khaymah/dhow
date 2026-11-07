package response

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"strconv"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
)

const fallbackErrorBody = `{"Success":false,"Error":{"Code":"INTERNAL","Message":"Something went wrong. Try again later."}}` + "\n"

type Meta struct {
	Page  int `json:"Page"`
	Limit int `json:"Limit"`
	Total int `json:"Total"`
}

type Envelope struct {
	Success bool                `json:"Success"`
	Data    any                 `json:"Data,omitempty"`
	Message string              `json:"Message,omitempty"`
	Meta    *Meta               `json:"Meta,omitempty"`
	Error   *apperrors.AppError `json:"Error,omitempty"`
}

func Success(w http.ResponseWriter, status int, data any, messageKey string, meta *Meta) {
	write(w, status, Envelope{Success: true, Data: data, Message: messageKey, Meta: meta})
}

func Error(w http.ResponseWriter, r *http.Request, ae *apperrors.AppError) {
	if ae == nil {
		ae = apperrors.Internal(nil)
	}
	if ae.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(math.Ceil(ae.RetryAfter.Seconds()))))
	}
	write(w, ae.Status, Envelope{Success: false, Error: Localize(r.Context(), ae)})
}

func Localize(ctx context.Context, ae *apperrors.AppError) *apperrors.AppError {
	locale := i18n.LocaleFrom(ctx)
	out := &apperrors.AppError{
		Status:   ae.Status,
		Code:     ae.Code,
		Message:  i18n.Translate(locale, ae.Message),
		Fix:      i18n.Fix(locale, ae.Code),
		DocsPath: i18n.DocsPath(locale, ae.Code),
	}
	for _, d := range ae.Details {
		out.Details = append(out.Details, apperrors.IErrorDetail{Field: d.Field, Message: i18n.Translate(locale, d.Message)})
	}
	return out
}

func write(w http.ResponseWriter, status int, env Envelope) {
	body, err := json.Marshal(env)
	if err != nil {
		status = http.StatusInternalServerError
		body = []byte(fallbackErrorBody)
	} else {
		body = append(body, '\n')
	}
	h := w.Header()
	h.Set("Content-Type", "application/json")
	if !env.Success {
		h.Set("Cache-Control", "no-store")
	}
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		return
	}
}
