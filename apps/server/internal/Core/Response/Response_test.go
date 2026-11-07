package response_test

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
	response "github.com/Khaymah/dhow/apps/server/internal/Core/Response"
)

type payload struct {
	Name string `json:"Name"`
}

func TestSuccess(t *testing.T) {
	tests := []struct {
		name string
		data any
		msg  string
		meta *response.Meta
		want string
	}{
		{"data only", payload{Name: "dhow"}, "", nil, `{"Success":true,"Data":{"Name":"dhow"}}`},
		{"data and message", payload{Name: "dhow"}, "system.meta.loaded", nil, `{"Success":true,"Data":{"Name":"dhow"},"Message":"system.meta.loaded"}`},
		{"with page meta", []int{1, 2}, "", &response.Meta{Page: 1, Limit: 2, Total: 9}, `{"Success":true,"Data":[1,2],"Meta":{"Page":1,"Limit":2,"Total":9}}`},
		{"nothing else", nil, "", nil, `{"Success":true}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			response.Success(rec, http.StatusOK, tt.data, tt.msg, tt.meta)
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			assert.JSONEq(t, tt.want, rec.Body.String())
		})
	}
}

func TestSuccessCreatedStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	response.Success(rec, http.StatusCreated, payload{Name: "x"}, "", nil)
	assert.Equal(t, http.StatusCreated, rec.Code)
}

func TestSuccessUnmarshalableFallsBack(t *testing.T) {
	rec := httptest.NewRecorder()
	response.Success(rec, http.StatusOK, make(chan int), "", nil)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.JSONEq(t, `{"Success":false,"Error":{"Code":"INTERNAL","Message":"Something went wrong. Try again later."}}`, rec.Body.String())
}

func TestError(t *testing.T) {
	tests := []struct {
		name       string
		acceptLang string
		err        *apperrors.AppError
		wantStatus int
		want       string
	}{
		{"arabic by default", "", apperrors.NotFound(), http.StatusNotFound, `{"Success":false,"Error":{"Code":"NOT_FOUND","Message":"المورد المطلوب غير موجود.","Fix":"تأكد من العنوان، أو ارجع إلى القائمة وافتح العنصر من جديد. ربما حُذف."}}`},
		{"english", "en", apperrors.NotFound(), http.StatusNotFound, `{"Success":false,"Error":{"Code":"NOT_FOUND","Message":"The requested resource does not exist.","Fix":"Check the address, or go back to the list and open the item again. It may have been removed."}}`},
		{"fix and docs path", "en", apperrors.CSRFFailed(), http.StatusForbidden, `{"Success":false,"Error":{"Code":"CSRF_FAILED","Message":"The request could not be verified. Reload the page and try again.","Fix":"Reload the page and try again. If it repeats, ask your server admin to check the public address and the reverse proxy.","DocsPath":"install/troubleshooting.md#setup-or-sign-in-fails-with-csrf_failed"}}`},
		{"no fix for an allowlisted code", "en", apperrors.MethodNotAllowed(), http.StatusMethodNotAllowed, `{"Success":false,"Error":{"Code":"METHOD_NOT_ALLOWED","Message":"This method is not allowed here."}}`},
		{
			"details are translated", "en",
			apperrors.ValidationFailed(apperrors.IErrorDetail{Field: "Name", Message: "errors.badRequest"}),
			http.StatusUnprocessableEntity,
			`{"Success":false,"Error":{"Code":"VALIDATION_FAILED","Message":"Some of the data is not valid. Check it and try again.","Fix":"Fix the fields marked in red, then send the form again.","Details":[{"Field":"Name","Message":"The request is not valid. Check the data and try again."}]}}`,
		},
		{"internal text never leaks", "en", apperrors.Internal(assert.AnError), http.StatusInternalServerError, `{"Success":false,"Error":{"Code":"INTERNAL","Message":"Something went wrong. Try again later.","Fix":"Try again in a minute. If it keeps happening, tell your server admin the time of the error so they can check the server log."}}`},
		{"nil error is internal", "en", nil, http.StatusInternalServerError, `{"Success":false,"Error":{"Code":"INTERNAL","Message":"Something went wrong. Try again later.","Fix":"Try again in a minute. If it keeps happening, tell your server admin the time of the error so they can check the server log."}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
			req = req.WithContext(i18n.WithLocale(req.Context(), i18n.Parse(tt.acceptLang)))
			rec := httptest.NewRecorder()
			response.Error(rec, req, tt.err)
			assert.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
			assert.Equal(t, "no-store", rec.Header().Get("Cache-Control"))
			assert.JSONEq(t, tt.want, rec.Body.String())
		})
	}
}

func TestErrorEnvelopeOmitsAbsentFields(t *testing.T) {
	rec := httptest.NewRecorder()
	response.Error(rec, httptest.NewRequest(http.MethodGet, "/", http.NoBody), apperrors.BadRequest())
	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &raw))
	assert.Len(t, raw, 2)
	assert.Contains(t, raw, "Success")
	assert.Contains(t, raw, "Error")
}

func TestLocalizeKeepsTheOriginal(t *testing.T) {
	ae := apperrors.NotFound()
	out := response.Localize(i18n.WithLocale(httptest.NewRequest(http.MethodGet, "/", http.NoBody).Context(), i18n.En), ae)
	assert.Equal(t, "errors.notFound", ae.Message)
	assert.Equal(t, "The requested resource does not exist.", out.Message)
	assert.Equal(t, http.StatusNotFound, out.Status)
	assert.Nil(t, out.Err)
}

func TestErrorSetsRetryAfter(t *testing.T) {
	tests := []struct {
		name  string
		after time.Duration
		want  string
	}{
		{"none", 0, ""},
		{"whole seconds", 30 * time.Second, "30"},
		{"rounded up", 1500 * time.Millisecond, "2"},
		{"minutes", 15 * time.Minute, "900"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			response.Error(rec, httptest.NewRequest(http.MethodPost, "/", http.NoBody), apperrors.InvalidCredentials(tt.after))
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
			assert.Equal(t, tt.want, rec.Header().Get("Retry-After"))
			assert.NotContains(t, rec.Body.String(), "RetryAfter")
		})
	}
}

func TestPageParams(t *testing.T) {
	ptr := func(n int) *int { return &n }
	tests := []struct {
		name      string
		page      *int
		limit     *int
		wantPage  int
		wantLimit int
	}{
		{"defaults", nil, nil, 1, 25},
		{"explicit", ptr(3), ptr(10), 3, 10},
		{"limit is capped", ptr(1), ptr(5000), 1, 100},
		{"zero and negative fall back", ptr(0), ptr(-4), 1, 25},
		{"page is capped so the offset fits in int32", ptr(math.MaxInt), ptr(100), config.MaxPage, 100},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, limit := response.PageParams(tt.page, tt.limit)
			assert.Equal(t, tt.wantPage, page)
			assert.Equal(t, tt.wantLimit, limit)
			assert.LessOrEqual(t, (page-1)*limit, math.MaxInt32, "the SQL offset is an int32")
		})
	}
}
