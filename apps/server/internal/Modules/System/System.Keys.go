package system

import (
	"context"
	"strconv"
	"strings"

	apperrors "github.com/Khaymah/dhow/apps/server/internal/Core/Errors"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	sqlc "github.com/Khaymah/dhow/apps/server/internal/Database/Sqlc"
	gen "github.com/Khaymah/dhow/apps/server/internal/Modules/System/Gen"
)

const (
	keysPath       = "/system/keys"
	messageKeyring = "system.keys.loaded"
)

type KeyringReader interface {
	Status(ctx context.Context) (keys.Status, error)
}

func (c *Controller) WithKeys(k KeyringReader) *Controller {
	c.keys = k
	return c
}

func (c *Controller) GetKeyringStatus(ctx context.Context, _ gen.GetKeyringStatusRequestObject) (gen.GetKeyringStatusResponseObject, error) {
	if c.keys == nil {
		return nil, apperrors.ServiceUnavailable()
	}
	st, err := c.keys.Status(ctx)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	data := keyringToGen(st)
	return gen.GetKeyringStatus200JSONResponse{Success: true, Message: ptr(messageKeyring), Data: &data}, nil
}

func keyringToGen(st keys.Status) gen.KeyringStatus {
	versions := make([]gen.KekVersionStatus, 0, len(st.Versions))
	for _, v := range st.Versions {
		versions = append(versions, gen.KekVersionStatus{
			Version: v.Version, State: gen.KekVersionStatusState(v.State), DataKeys: v.DataKeys, Loaded: v.Local,
			ActivatedAt: v.ActivatedAt, RetiredAt: v.RetiredAt, LastExportedAt: v.LastExportedAt,
		})
	}
	loaded := st.LocalVersions
	if loaded == nil {
		loaded = []int32{}
	}
	return gen.KeyringStatus{
		Provider: st.Provider, ActiveVersion: st.Active, Versions: versions, ToRewrap: st.ToRewrap, LoadedVersions: loaded,
		LastRotatedAt: st.LastRotatedAt, LastExportedAt: st.LastExportedAt,
	}
}

func KeysActivatedEvent(from, to int32) eventbus.Event {
	return keysEvent(eventbus.SystemKeysActivated, map[string]string{"From": versionText(from), "To": versionText(to)})
}

func KeysExportedEvent(versions []int32) eventbus.Event {
	return keysEvent(eventbus.SystemKeysExported, map[string]string{"KekVersions": versionsText(versions)})
}

func KeysDiscardedEvent(tx *sqlc.Queries, versions []int32, dataKeys int64) eventbus.Event {
	ev := keysEvent(eventbus.SystemKeysDiscarded, map[string]string{
		"KekVersions": versionsText(versions), "DataKeys": strconv.FormatInt(dataKeys, 10), eventbus.SeverityKey: eventbus.SeverityHigh,
	})
	if a, ok := ev.Payload.(eventbus.Audited); ok {
		a.Tx = tx
		ev.Payload = a
	}
	return ev
}

func keysEvent(name string, data map[string]string) eventbus.Event {
	return eventbus.Event{Name: name, Payload: eventbus.Audited{TargetType: "keyring", Data: data}}
}

func versionText(v int32) string { return strconv.FormatInt(int64(v), 10) }

func versionsText(vs []int32) string {
	parts := make([]string, 0, len(vs))
	for _, v := range vs {
		parts = append(parts, versionText(v))
	}
	return strings.Join(parts, ",")
}
