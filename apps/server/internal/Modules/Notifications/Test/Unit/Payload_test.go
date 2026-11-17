package unit_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	i18n "github.com/Khaymah/dhow/apps/server/internal/Core/I18n"
	notifications "github.com/Khaymah/dhow/apps/server/internal/Modules/Notifications"
)

func TestThePayloadHasOnlyTheAllowedFieldsInPascalCase(t *testing.T) {
	allowed := []string{
		"Event", "EventId", "Time", "Status", "Reason", "Project", "Environment", "Ref", "Run", "Release", "Runner", "Change", "Browser", "Network", "Url",
	}
	typ := reflect.TypeOf(notifications.Payload{})
	var got []string
	for i := range typ.NumField() {
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		got = append(got, name)
	}
	assert.Equal(t, allowed, got, "a new field is a new thing that leaves the server: add it here on purpose")
}

func TestThePayloadEncodesWithoutLogLinesOrControlCharacters(t *testing.T) {
	raw, err := notifications.Payload{
		Event: notifications.EventDeploymentFinished, EventID: uuid.NewString(), Time: "2026-11-07T10:00:00Z", Status: notifications.StatusFailed,
		Project: "shop\n<!channel> @everyone", Ref: strings.Repeat("a", 500),
	}.Encode()
	require.NoError(t, err)
	var back map[string]any
	require.NoError(t, json.Unmarshal(raw, &back))
	assert.Equal(t, "shop <!channel> @everyone", back["Project"], "line breaks become spaces; the text is kept literally")
	assert.Len(t, []rune(back["Ref"].(string)), 120)
	assert.NotContains(t, string(raw), "\\n")
}

func TestARuleMatchesOnEventEnvironmentAndStatus(t *testing.T) {
	rule := notifications.RuleSpec{
		Events: notifications.DefaultEvents(), Environments: []string{"production"}, Statuses: notifications.DefaultStatuses(),
	}
	tests := []struct {
		name   string
		notice notifications.Notice
		want   bool
	}{
		{"failed in production", notifications.Notice{Event: notifications.EventDeploymentFinished, Status: notifications.StatusFailed, Environment: "production"}, true},
		{"rolled back in production", notifications.Notice{Event: notifications.EventDeploymentFinished, Status: notifications.StatusRolledBack, Environment: "production"}, true},
		{"rollback failed", notifications.Notice{Event: notifications.EventDeploymentFinished, Status: notifications.StatusRollbackFailed, Environment: "production"}, true},
		{"approval", notifications.Notice{Event: notifications.EventApprovalRequested, Status: notifications.StatusAwaitingApprove, Environment: "production"}, true},
		{"success is not in the default", notifications.Notice{Event: notifications.EventDeploymentFinished, Status: notifications.StatusSucceeded, Environment: "production"}, false},
		{"another environment", notifications.Notice{Event: notifications.EventDeploymentFinished, Status: notifications.StatusFailed, Environment: "staging"}, false},
		{"no environment on an environment rule", notifications.Notice{Event: notifications.EventDeploymentFinished, Status: notifications.StatusFailed}, false},
		{"an event the rule does not list", notifications.Notice{Event: notifications.EventRunnerOffline}, false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, rule.Matches(tt.notice), tt.name)
	}
	any := notifications.RuleSpec{Events: []string{notifications.EventRunnerOffline}, Statuses: notifications.DefaultStatuses()}
	assert.True(t, any.Matches(notifications.Notice{Event: notifications.EventRunnerOffline}), "an event without a status or an environment")
}

func TestTheDefaultRuleLeavesOutSuccess(t *testing.T) {
	assert.NotContains(t, notifications.DefaultStatuses(), notifications.StatusSucceeded)
	assert.ElementsMatch(t, []string{
		notifications.StatusFailed, notifications.StatusRolledBack, notifications.StatusRollbackFailed, notifications.StatusAwaitingApprove,
	}, notifications.DefaultStatuses())
}

func TestRenderMailIsInTheLanguageOfTheChannelAndLeavesNoPlaceholder(t *testing.T) {
	p := notifications.Payload{
		Event: notifications.EventDeploymentFinished, Status: notifications.StatusFailed, Project: "shop", Environment: "production",
		Ref: "main", Run: 12, Time: "2026-11-07T10:00:00Z", URL: "https://dhow.example.com/projects/p/runs/12",
	}
	subject, body := notifications.RenderMail(i18n.Ar, p)
	assert.Contains(t, subject, "فشل النشر")
	assert.Contains(t, subject, "shop")
	assert.Contains(t, body, "الحالة: فشل")
	assert.Contains(t, body, "رقم التنفيذ: 12")
	subject, body = notifications.RenderMail(i18n.En, p)
	assert.Contains(t, subject, "Deployment failed")
	assert.Contains(t, body, "Environment: production")
	assert.Contains(t, body, p.URL)
	for _, text := range []string{subject, body} {
		assert.NotContains(t, text, "{")
		assert.NotContains(t, text, "notify.")
	}
	assert.NotContains(t, subject, "\n")
	_, runnerBody := notifications.RenderMail(i18n.En, notifications.Payload{Event: notifications.EventRunnerOffline, Runner: "box-1"})
	assert.Contains(t, runnerBody, "Runner: box-1")
	assert.NotContains(t, runnerBody, "Project:", "a field without a value gets no line")
}

func TestEveryEventAndStatusHasAMessageInBothLanguages(t *testing.T) {
	for _, locale := range []i18n.Locale{i18n.Ar, i18n.En} {
		for _, event := range notifications.Events() {
			status := ""
			if event == notifications.EventDeploymentFinished {
				for _, s := range notifications.Statuses() {
					if s != notifications.StatusAwaitingApprove {
						_, body := notifications.RenderMail(locale, notifications.Payload{Event: event, Status: s})
						assert.NotContains(t, body, "notify.title", "%s %s %s", locale, event, s)
					}
				}
				continue
			}
			_, body := notifications.RenderMail(locale, notifications.Payload{Event: event, Status: status})
			assert.NotContains(t, body, "notify.title", "%s %s", locale, event)
		}
	}
}
