package app

import (
	"context"
	"errors"
	"time"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	system "github.com/Khaymah/dhow/apps/server/internal/Modules/System"
)

const reasonUnknown = "UNKNOWN"

func (k KeysRunner) rekey(ctx context.Context) error {
	provider, err := k.provider()
	if err != nil {
		return err
	}
	pool, err := k.connect(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	manager := keys.NewManager(pool, provider, k.logger())
	if err := manager.Verify(ctx); err != nil {
		return err
	}
	reg, err := ValueRegistry()
	if err != nil {
		return err
	}
	if err := k.record(ctx, pool, system.KeysRekeyStartedEvent()); err != nil {
		return err
	}
	k.say("Replacing the data key of every owner and sealing every stored value again. This is safe to stop and run again.")
	res, err := manager.Rekey(ctx, pool, keys.NewValues(manager.Envelope()), reg, keys.RekeyOptions{Grace: k.Opts.Grace})
	if res.Resumed {
		k.say("Resumed the rekey that started before.")
	}
	if err != nil {
		k.say("Stopped after %d new data keys and %d values sealed again. Run the same command to continue.", res.Superseded, res.Resealed)
		code := keys.CodeOf(err)
		if code == "" {
			code = reasonUnknown
		}
		failed := system.KeysRekeyFailedEvent(code, res.Superseded, res.Resealed, res.Destroyed)
		return errors.Join(err, k.record(context.WithoutCancel(ctx), pool, failed))
	}
	if err := k.record(ctx, pool, system.KeysRekeyedEvent(res.Superseded, res.Resealed, res.Destroyed)); err != nil {
		return err
	}
	k.say("Done: %d owners have a new data key, %d values were sealed again and %d old data keys were destroyed.",
		res.Superseded, res.Resealed, res.Destroyed)
	k.sayKekState(res)
	k.say("The secret values themselves did not change. To replace a secret that leaked, change it where it is used and save it again.")
	return nil
}

func (k KeysRunner) sayKekState(res keys.RekeyResult) {
	if res.KekActivatedAt != nil && res.Cutoff.Sub(*res.KekActivatedAt) <= config.KeyRekeyFreshKek {
		k.say("The new data keys are wrapped with KEK version %d, activated just before this rekey: a database copy from before it, with the keyring it had, opens none of the current values.", res.KekVersion)
		return
	}
	since := "an unknown time"
	if res.KekActivatedAt != nil {
		since = res.KekActivatedAt.UTC().Format(time.RFC3339)
	}
	k.say("WARNING: the new data keys are wrapped with KEK version %d, active since %s, long before this rekey. A keyring that leaked with that version still unwraps them. If the keyring leaked, run dhow-server keys generate, copy the keyring to every server, restart them, run dhow-server keys activate, then run this rekey again.", res.KekVersion, since)
}
