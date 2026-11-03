package installer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"

	keys "github.com/Khaymah/dhow/apps/server/internal/Core/Keys"
)

func (b *Builder) nativeSteps() []Step {
	var steps []Step
	if b.A.Database.Mode == DatabaseInstall {
		steps = append(steps, b.postgresSteps()...)
	}
	steps = append(steps,
		b.userStep(),
		DirStep(b.Sys, b.M.T(KeyStepConfigDir), ConfigDir, modeRootDir),
		DirStep(b.Sys, b.M.T(KeyStepCredentialsDir), CredentialsDir, modeRootDir),
		b.track(SecretFileStep(b.Sys, b.M.T(KeyStepSecretKey), SecretKeyPath, generateSecretKey)),
		b.track(SecretFileStep(b.Sys, b.M.T(KeyStepKeyring), KeyringPath, generateKeyring)),
	)
	if b.A.Database.Mode == DatabaseInstall {
		steps = append(steps, b.track(b.databaseStep()))
	} else {
		url := b.A.DatabaseURL()
		steps = append(steps, b.track(SecretFileStep(b.Sys, b.M.T(KeyStepDatabaseURL), DatabaseURLPath,
			func() ([]byte, error) { return []byte(url + "\n"), nil })))
	}
	steps = append(steps,
		b.track(FileStep(b.Sys, b.M.T(KeyStepEnvFile), EnvFilePath, nativeEnv(b.A, ParseEnv(b.readExisting(EnvFilePath))), modeRootPrivate)),
		b.track(b.binaryStep()),
		b.track(b.unitStep()),
		b.answersStep(),
		b.startStep(),
	)
	return steps
}

func generateSecretKey() ([]byte, error) {
	key, err := RandomHex(SecretKeyBytes)
	if err != nil {
		return nil, err
	}
	return []byte(key + "\n"), nil
}

func generateKeyring() ([]byte, error) {
	ring, err := keys.GenerateKeyring(1)
	if err != nil {
		return nil, err
	}
	defer ring.Destroy()
	return ring.Marshal(), nil
}

func (b *Builder) userStep() Step {
	add := Cmd("useradd", "--system", "--user-group", "--no-create-home", "--home-dir", StateDir,
		"--shell", b.Det.Distro.NologinShell(), "--comment", "Dhow service", ServiceUser)
	check := Cmd("getent", "passwd", ServiceUser)
	return CommandStep(b.Sys, b.M.T(KeyStepUser, ServiceUser), &check, []Command{add}, []Command{Cmd("userdel", ServiceUser)})
}

func (b *Builder) answersStep() Step {
	data, err := b.A.Marshal()
	if err != nil {
		return Step{Title: b.M.T(KeyStepAnswers), Apply: func(context.Context) error { return err }}
	}
	return FileStep(b.Sys, b.M.T(KeyStepAnswers), AnswersPath, data, modeRootPrivate)
}

func (b *Builder) binaryStep() Step {
	var backup []byte
	return Step{
		Title: b.M.T(KeyStepBinary, b.Version),
		Files: []string{BinaryPath},
		Done: func(context.Context) (bool, error) {
			if b.Exe == BinaryPath {
				return true, nil
			}
			current, err := b.Sys.ReadFile(BinaryPath)
			if err != nil {
				return false, nil
			}
			self, err := b.Sys.ReadFile(b.Exe)
			if err != nil {
				return false, err
			}
			info, err := b.Sys.Stat(BinaryPath)
			if err != nil {
				return false, err
			}
			return sha256.Sum256(current) == sha256.Sum256(self) && info.Mode.Perm() == modeRootExec.Perm, nil
		},
		Apply: func(context.Context) error {
			self, err := b.Sys.ReadFile(b.Exe)
			if err != nil {
				return err
			}
			if old, err := b.Sys.ReadFile(BinaryPath); err == nil {
				backup = old
			}
			return b.Sys.WriteFile(BinaryPath, self, modeRootExec)
		},
		Undo: func(context.Context) error {
			if backup != nil {
				return b.Sys.WriteFile(BinaryPath, backup, modeRootExec)
			}
			return b.Sys.Remove(BinaryPath)
		},
	}
}

func (b *Builder) unitStep() Step {
	files := []fileSpec{{Path: UnitPath, Data: UnitFile, Mode: modeRootPublic}}
	s := b.ConfigStep(b.M.T(KeyStepUnit), files, []Command{Cmd("systemctl", "daemon-reload")},
		func(context.Context) (bool, error) { return b.filesMatch(files) })
	undo := s.Undo
	s.Undo = func(ctx context.Context) error {
		err := undo(ctx)
		_, rerr := b.Sys.Run(ctx, Cmd("systemctl", "daemon-reload"))
		return errors.Join(err, rerr)
	}
	return s
}

func (b *Builder) startStep() Step {
	enable := Cmd("systemctl", "enable", ServiceName)
	restart := Cmd("systemctl", "restart", ServiceName)
	return Step{
		Title:    b.M.T(KeyStepStart),
		Commands: []string{enable.String(), restart.String()},
		Done: func(ctx context.Context) (bool, error) {
			if b.restart || !b.serviceActive(ctx, ServiceName) {
				return false, nil
			}
			res, err := b.Sys.HTTP(ctx, Probe{URL: "http://" + upstream(b.A) + "/api/v1/health/ready"})
			return err == nil && res.Status == 200, nil
		},
		Apply: func(ctx context.Context) error {
			if _, err := run(ctx, b.Sys, enable); err != nil {
				return err
			}
			if _, err := run(ctx, b.Sys, restart); err != nil {
				return errors.Join(err, b.journalTail(ctx))
			}
			if err := b.waitReady(ctx); err != nil {
				return errors.Join(err, b.journalTail(ctx))
			}
			return nil
		},
	}
}

func (b *Builder) journalTail(ctx context.Context) error {
	out, err := b.Sys.Run(ctx, Cmd("journalctl", "--unit", ServiceName, "--lines", "25", "--no-pager", "--output", "cat"))
	if err != nil || len(bytes.TrimSpace(out.Stdout)) == 0 {
		return nil
	}
	return errors.New(b.M.T(KeyJournalTail) + "\n" + string(bytes.TrimSpace(out.Stdout)))
}
