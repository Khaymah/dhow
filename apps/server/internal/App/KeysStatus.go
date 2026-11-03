package app

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"time"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	system "github.com/Khaymah/dhow/apps/server/internal/Modules/System"
)

const (
	timeLayout = "2006-01-02 15:04 UTC"
	never      = "never"
	yes        = "yes"
	no         = "no"
)

func (k KeysRunner) status(ctx context.Context) error {
	provider, err := k.provider()
	if err != nil {
		return err
	}
	pool, err := k.connect(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	st, err := keys.NewManager(pool, provider, k.logger()).Status(ctx)
	if err != nil {
		return err
	}
	k.printStatus(st, time.Now())
	return nil
}

func (k KeysRunner) printStatus(st keys.Status, now time.Time) {
	p := newPrinter(k.Out)
	p.row("Provider:\t%s", st.Provider)
	p.row("Active KEK version:\t%s", activeText(st.Active))
	p.row("Data keys to re-wrap:\t%d", st.ToRewrap)
	p.row("Last rotation:\t%s", when(st.LastRotatedAt))
	p.row("Last export:\t%s", when(st.LastExportedAt))
	p.row("")
	p.row("VERSION\tSTATE\tDATA KEYS\tIN THIS FILE\tACTIVATED\tRETIRED\tEXPORTED")
	for _, v := range st.Versions {
		p.row("%d\t%s\t%d\t%s\t%s\t%s\t%s", v.Version, v.State, v.DataKeys, yesNo(v.Local), when(v.ActivatedAt), when(v.RetiredAt), when(v.LastExportedAt))
	}
	if err := p.flush(); err != nil {
		return
	}
	for _, v := range st.Versions {
		if v.State == keys.KekRetired && v.Local {
			k.say("Version %d is retired and no data key uses it. Remove its line from the keyring file once every backup taken before %s is gone,"+
				" because each backup needs the keyring it was taken with.", v.Version, when(v.RetiredAt))
		}
	}
	for _, w := range statusWarnings(st, now) {
		k.say("Warning: %s", w)
	}
}

func statusWarnings(st keys.Status, now time.Time) []string {
	var out []string
	if st.LastExportedAt == nil {
		out = append(out, "the keyring was never exported; run dhow-server keys export --recipient age1... --out FILE and keep the copy off this server")
	}
	for _, v := range st.Versions {
		if v.State == keys.KekActive && v.ActivatedAt != nil && now.Sub(*v.ActivatedAt) > config.KekActiveMaxAge {
			out = append(out, fmt.Sprintf("KEK version %d has been active for more than a year; rotate it with dhow-server keys generate", v.Version))
		}
	}
	return out
}

func activeText(v int32) string {
	if v == 0 {
		return "none yet"
	}
	return fmt.Sprint(v)
}

func when(t *time.Time) string {
	if t == nil {
		return never
	}
	return t.UTC().Format(timeLayout)
}

func yesNo(b bool) string {
	if b {
		return yes
	}
	return no
}

func (k KeysRunner) discard(ctx context.Context) error {
	kr, err := k.keyring()
	if err != nil {
		return err
	}
	if _, err := os.Lstat(kr.Path); errors.Is(err, fs.ErrNotExist) {
		if err := k.init(); err != nil {
			return err
		}
	}
	provider, err := OpenKeyring(kr)
	if err != nil {
		return err
	}
	pool, err := k.connect(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	manager := keys.NewManager(pool, provider, k.logger())
	plan, err := manager.PlanDiscard(ctx)
	if err != nil {
		return err
	}
	if plan.Empty() {
		k.say("Every KEK version the database needs is in %s with a matching check value. Nothing to discard.", kr.Path)
		return nil
	}
	k.describeDiscard(plan)
	if k.Opts.Confirm != DiscardConfirmation {
		return ErrDiscardUnconfirm
	}
	if plan, err = manager.Discard(ctx); err != nil {
		return err
	}
	if err := k.record(ctx, pool, system.KeysDiscardedEvent(plan.Versions(), plan.DataKeys)); err != nil {
		return err
	}
	reg, err := manager.Register(ctx)
	if err != nil {
		return err
	}
	k.say("Discarded %d data keys. KEK version %d is active. dhow-server can start now.", plan.DataKeys, reg.Active)
	k.say("Enter the lost secrets again, and ask people with an authenticator app to set it up again.")
	return nil
}

func (k KeysRunner) describeDiscard(plan keys.DiscardPlan) {
	k.say("This database was not made with this keyring:")
	if len(plan.Missing) > 0 {
		k.say("  missing from the keyring file: KEK versions %s", joinVersions(plan.Missing))
	}
	if len(plan.Mismatched) > 0 {
		k.say("  a different key under the same number: KEK versions %s", joinVersions(plan.Mismatched))
	}
	k.say("%d data keys can never be opened again. Every value sealed with them will count as erased.", plan.DataKeys)
	k.say("If you still have the right keyring file, or its export, restore it instead and start dhow-server.")
}

func joinVersions(vs []int32) string {
	parts := make([]string, 0, len(vs))
	for _, v := range vs {
		parts = append(parts, fmt.Sprint(v))
	}
	return strings.Join(parts, ", ")
}
