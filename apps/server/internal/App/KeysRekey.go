package app

import (
	"context"

	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	system "github.com/Khaymah/dhow/apps/server/internal/Modules/System"
)

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
	k.say("Replacing the data key of every owner and sealing every stored value again. This is safe to stop and run again.")
	res, err := manager.Rekey(ctx, pool, keys.NewValues(manager.Envelope(), nil), reg, keys.RekeyOptions{Grace: k.Opts.Grace})
	if res.Resumed {
		k.say("Resumed the rekey that started before.")
	}
	if err != nil {
		k.say("Stopped after %d new data keys and %d values sealed again. Run the same command to continue.", res.Superseded, res.Resealed)
		return err
	}
	if err := k.record(ctx, pool, system.KeysRekeyedEvent(res.Superseded, res.Resealed, res.Destroyed)); err != nil {
		return err
	}
	k.say("Done: %d owners have a new data key, %d values were sealed again and %d old data keys were destroyed.",
		res.Superseded, res.Resealed, res.Destroyed)
	k.say("A copy of the database and keyring from before this rekey opens none of the current values.")
	return nil
}
