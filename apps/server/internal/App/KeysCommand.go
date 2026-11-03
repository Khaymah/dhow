package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"filippo.io/age"
	"filippo.io/age/armor"
	"github.com/jackc/pgx/v5/pgxpool"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
	eventbus "github.com/Khaymah/dhow/apps/server/internal/Core/EventBus"
	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
	audit "github.com/Khaymah/dhow/apps/server/internal/Modules/Audit"
	system "github.com/Khaymah/dhow/apps/server/internal/Modules/System"
)

const (
	CommandKeys    = "keys"
	KeysInit       = "init"
	KeysStatus     = "status"
	KeysGenerate   = "generate"
	KeysActivate   = "activate"
	KeysExport     = "export"
	KeysRekey      = "rekey"
	stdoutPath     = "-"
	exportFilePerm = 0o600
	keysUsage      = "usage: dhow-server keys init [--discard-unreadable [--confirm destroy-N-keys]] | status | generate | " +
		"activate VERSION | export --recipient age1... --out FILE|- | rekey [--grace 10s, at least 5s]; every command takes --file PATH, and the default is " +
		config.EnvKeyringFile + " or the systemd credential " + config.CredentialKeyring
)

var (
	ErrKeysUsage        = errors.New(keysUsage)
	ErrDiscardUnconfirm = errors.New("nothing changed; add the --confirm value shown above, which names the number of data keys to destroy")
)

type KeysOptions struct {
	Command   string
	File      string
	Discard   bool
	Confirm   string
	Version   int32
	Recipient string
	Out       string
	Grace     time.Duration
}

func ParseKeysArgs(args []string) (KeysOptions, error) {
	if len(args) == 0 {
		return KeysOptions{}, ErrKeysUsage
	}
	opts := KeysOptions{Command: args[0]}
	flags := flag.NewFlagSet(CommandKeys+" "+opts.Command, flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&opts.File, "file", "", "")
	switch opts.Command {
	case KeysInit:
		flags.BoolVar(&opts.Discard, "discard-unreadable", false, "")
		flags.StringVar(&opts.Confirm, "confirm", "", "")
	case KeysExport:
		flags.StringVar(&opts.Recipient, "recipient", "", "")
		flags.StringVar(&opts.Out, "out", "", "")
	case KeysRekey:
		flags.DurationVar(&opts.Grace, "grace", config.KeyRekeyGrace, "")
	case KeysStatus, KeysGenerate, KeysActivate:
	default:
		return KeysOptions{}, ErrKeysUsage
	}
	rest := args[1:]
	if opts.Command == KeysActivate {
		if len(rest) == 0 {
			return KeysOptions{}, ErrKeysUsage
		}
		v, err := strconv.ParseInt(rest[0], 10, 32)
		if err != nil || v < 1 {
			return KeysOptions{}, ErrKeysUsage
		}
		opts.Version, rest = int32(v), rest[1:]
	}
	if err := flags.Parse(rest); err != nil || flags.NArg() != 0 {
		return KeysOptions{}, ErrKeysUsage
	}
	if (opts.Command == KeysExport && (opts.Recipient == "" || opts.Out == "")) || (opts.Confirm != "" && !opts.Discard) || (opts.Command == KeysRekey && opts.Grace < config.KeyRekeyMinGrace) {
		return KeysOptions{}, ErrKeysUsage
	}
	return opts, nil
}

type KeysRunner struct {
	Opts    KeysOptions
	Out     io.Writer
	Stdout  io.Writer
	Getenv  func(string) string
	Connect func(ctx context.Context) (*pgxpool.Pool, error)
	Log     *slog.Logger
}

func RunKeys(ctx context.Context, args []string, out io.Writer) error {
	opts, err := ParseKeysArgs(args)
	if err != nil {
		return err
	}
	return KeysRunner{Opts: opts, Out: out, Stdout: os.Stdout, Getenv: os.Getenv}.Run(ctx)
}

func (k KeysRunner) Run(ctx context.Context) error {
	switch k.Opts.Command {
	case KeysInit:
		if k.Opts.Discard {
			return k.discard(ctx)
		}
		return k.init()
	case KeysStatus:
		return k.status(ctx)
	case KeysGenerate:
		return k.generate()
	case KeysActivate:
		return k.activate(ctx)
	case KeysExport:
		return k.export(ctx)
	case KeysRekey:
		return k.rekey(ctx)
	}
	return ErrKeysUsage
}

func (k KeysRunner) keyring() (config.Keyring, error) {
	if k.Opts.File == "" {
		return config.LoadKeyring(k.Getenv)
	}
	abs, err := filepath.Abs(k.Opts.File)
	if err != nil {
		return config.Keyring{}, fmt.Errorf("--file: %w", err)
	}
	return config.Keyring{Path: abs}, nil
}

func (k KeysRunner) provider() (*keys.FileProvider, error) {
	kr, err := k.keyring()
	if err != nil {
		return nil, err
	}
	return OpenKeyring(kr)
}

func (k KeysRunner) connect(ctx context.Context) (*pgxpool.Pool, error) {
	if k.Connect != nil {
		return k.Connect(ctx)
	}
	cfg, err := config.LoadMigrate(k.Getenv)
	if err != nil {
		return nil, fmt.Errorf("invalid configuration:\n%w", err)
	}
	return config.OpenDatabase(ctx, cfg.DatabaseURL)
}

func (k KeysRunner) logger() *slog.Logger {
	if k.Log != nil {
		return k.Log
	}
	return slog.New(slog.DiscardHandler)
}

func (k KeysRunner) say(format string, args ...any) {
	_, _ = fmt.Fprintf(k.Out, format+"\n", args...)
}

func (k KeysRunner) record(ctx context.Context, pool *pgxpool.Pool, ev eventbus.Event) error {
	if err := audit.NewService(pool, k.logger(), time.Now).Handle(ctx, ev); err != nil {
		return fmt.Errorf("write the audit entry: %w", err)
	}
	return nil
}

func (k KeysRunner) init() error {
	kr, err := k.keyring()
	if err != nil {
		return err
	}
	ring, err := keys.GenerateKeyring(1)
	if err != nil {
		return err
	}
	defer ring.Destroy()
	if err := keys.CreateKeyringFile(kr.Path, ring); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s already exists and is never overwritten: it may be the only copy of the keys that open your secrets", kr.Path)
		}
		return err
	}
	k.say("Wrote a new keyring with KEK version 1 to %s (mode 600).", kr.Path)
	k.say("Export it now and keep the copy away from this server: without the keyring every stored secret is lost.")
	k.say("  dhow-server keys export --recipient age1... --out dhow-keyring.age")
	return nil
}

func (k KeysRunner) generate() error {
	kr, err := k.keyring()
	if err != nil {
		return err
	}
	version, err := keys.AppendKeyringVersion(kr.Path, keys.FileRules{AllowGroupRead: kr.Credential})
	if err != nil {
		return err
	}
	k.say("Added KEK version %d to %s. Nothing uses it yet.", version, kr.Path)
	k.say("Next:")
	k.say("  1. Copy this keyring file to every server that runs dhow-server.")
	k.say("  2. Restart them one at a time; each one registers version %d as pending.", version)
	k.say("  3. Run: dhow-server keys activate %d", version)
	return nil
}

func (k KeysRunner) activate(ctx context.Context) error {
	provider, err := k.provider()
	if err != nil {
		return err
	}
	pool, err := k.connect(ctx)
	if err != nil {
		return err
	}
	defer pool.Close()
	act, err := keys.NewManager(pool, provider, k.logger()).Activate(ctx, k.Opts.Version, config.ServerInstanceStaleAfter.Seconds())
	if err != nil {
		return err
	}
	if err := k.record(ctx, pool, system.KeysActivatedEvent(act.From, act.To)); err != nil {
		return err
	}
	k.say("KEK version %d is active. New data keys are wrapped with it now.", act.To)
	if act.From != 0 {
		k.say("Version %d is retiring: the servers re-wrap its data keys in the background, and it turns retired when none is left.", act.From)
	}
	k.say("Follow the progress with: dhow-server keys status")
	return nil
}

func (k KeysRunner) export(ctx context.Context) error {
	recipient, err := keys.ParseRecipient(k.Opts.Recipient)
	if err != nil {
		return err
	}
	kr, err := k.keyring()
	if err != nil {
		return err
	}
	ring, err := keys.ReadKeyringFile(kr.Path, keys.FileRules{AllowGroupRead: kr.Credential})
	if err != nil {
		return err
	}
	defer ring.Destroy()
	if err := k.writeExport(ring, recipient); err != nil {
		return err
	}
	pool, err := k.connect(ctx)
	if err != nil {
		return fmt.Errorf("the encrypted copy was written, but the export could not be recorded: %w", err)
	}
	defer pool.Close()
	manager := keys.NewManager(pool, keys.KeyProvider(nil), k.logger())
	if err := manager.MarkExported(ctx, ring.Versions()); err != nil {
		return fmt.Errorf("the encrypted copy was written, but %w", err)
	}
	if err := k.record(ctx, pool, system.KeysExportedEvent(ring.Versions())); err != nil {
		return err
	}
	if k.Opts.Out != stdoutPath {
		k.say("Wrote the keyring, encrypted to %s, to %s. Only the matching age private key opens it; keep that key off this server.",
			k.Opts.Recipient, k.Opts.Out)
	}
	return nil
}

func (k KeysRunner) writeExport(ring *keys.Keyring, recipient age.Recipient) error {
	dst := k.Stdout
	var file *os.File
	if k.Opts.Out != stdoutPath {
		f, err := os.OpenFile(k.Opts.Out, os.O_WRONLY|os.O_CREATE|os.O_EXCL|syscall.O_NOFOLLOW, exportFilePerm)
		if err != nil {
			return fmt.Errorf("create the export file: %w", err)
		}
		file, dst = f, f
	}
	aw := armor.NewWriter(dst)
	err := keys.Export(aw, ring, recipient)
	if err == nil {
		err = aw.Close()
	}
	if file != nil {
		if err == nil {
			err = file.Sync()
		}
		if cerr := file.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			_ = os.Remove(k.Opts.Out)
		}
	}
	if err != nil {
		return fmt.Errorf("write the export: %w", err)
	}
	return nil
}
