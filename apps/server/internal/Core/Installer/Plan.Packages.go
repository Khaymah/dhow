package installer

import (
	"context"
	"slices"
	"strings"

	engine "github.com/Khaymah/dhow/internal/installer"
)

func (b *Builder) packageSteps() []Step {
	missing := MissingPackages(b.Det, b.A)
	if len(missing) == 0 || !b.Det.PackagesInstallable() || !b.A.InstallPackages {
		return nil
	}
	needPGDG := slices.Contains(missing, b.Det.Distro.PostgresPackage())
	if b.Det.Distro.Family == FamilyApt {
		return b.aptSteps(missing, needPGDG)
	}
	return b.dnfSteps(missing, needPGDG)
}

func (b *Builder) aptInstall(title string, pkgs []string) Step {
	return engine.AptInstallStep(b.Sys, title, pkgs)
}

func (b *Builder) aptSteps(missing []string, needPGDG bool) []Step {
	var steps []Step
	if needPGDG {
		steps = append(steps,
			b.aptInstall(b.M.T(KeyStepCACerts), []string{"ca-certificates"}),
			b.keyStep(AptKeyURL, AptKeyFinger, AptKeyPath, nil),
			FileStep(b.Sys, b.M.T(KeyStepPGDGRepo), AptSourcePath, aptSource(b.Det.Distro), modeRootPublic),
		)
	}
	return append(steps, b.aptInstall(b.M.T(KeyStepPackages), missing))
}

func aptSource(d Distro) []byte {
	return []byte("Types: deb\n" +
		"URIs: https://apt.postgresql.org/pub/repos/apt\n" +
		"Suites: " + d.Codename + "-pgdg\n" +
		"Components: main\n" +
		"Signed-By: " + AptKeyPath + "\n")
}

func dnfRepo(d Distro) []byte {
	base := DnfRepoBase + "redhat/rhel-$releasever-$basearch"
	if d.Fedora {
		base = DnfRepoBase + "fedora/fedora-$releasever-$basearch"
	}
	return []byte("[dhow-pgdg18]\n" +
		"name=PostgreSQL 18 from the PostgreSQL Global Development Group\n" +
		"baseurl=" + base + "\n" +
		"enabled=1\n" +
		"gpgcheck=1\n" +
		"repo_gpgcheck=1\n" +
		"gpgkey=file://" + DnfKeyPath + "\n")
}

func (b *Builder) dnfInstall(title string, pkgs []string) Step {
	return engine.DnfInstallStep(b.Sys, title, pkgs)
}

func (b *Builder) dnfSteps(missing []string, needPGDG bool) []Step {
	var steps []Step
	d := b.Det.Distro
	needsExtra := slices.ContainsFunc(missing, func(p string) bool { return p == "caddy" || p == "certbot" })
	if needsExtra && d.NeedsEPEL() {
		steps = append(steps, b.dnfInstall(b.M.T(KeyStepEPEL), []string{"epel-release"}))
	}
	if needPGDG {
		importKey := Cmd("rpm", "--import", DnfKeyPath)
		steps = append(steps,
			b.keyStep(DnfKeyURL, DnfKeyFinger, DnfKeyPath, &importKey),
			FileStep(b.Sys, b.M.T(KeyStepPGDGRepo), DnfRepoPath, dnfRepo(d), modeRootPublic),
		)
		if d.EL {
			steps = append(steps, CommandStep(b.Sys, b.M.T(KeyStepModuleDisable), nil,
				[]Command{Cmd("dnf", "-qy", "module", "disable", "postgresql")}, nil))
		}
		if d.Fedora {
			steps = append(steps,
				DirStep(b.Sys, b.M.T(KeyStepDnfDropInDir), parentDir(DnfExcludePath), modePublicDir),
				FileStep(b.Sys, b.M.T(KeyStepFedoraExclude), DnfExcludePath, []byte("[main]\nexcludepkgs=postgresql,postgresql-*\n"), modeRootPublic))
		}
	}
	return append(steps, b.dnfInstall(b.M.T(KeyStepPackages), missing))
}

func (b *Builder) keyStep(url, fingerprint, path string, after *Command) Step {
	s := Step{
		Title: b.M.T(KeyStepRepoKey, fingerprint),
		Files: []string{path},
		Done: func(context.Context) (bool, error) {
			data, err := b.Sys.ReadFile(path)
			if err != nil {
				return false, nil
			}
			return VerifyKeyFingerprint(data, fingerprint) == nil, nil
		},
		Apply: func(ctx context.Context) error {
			data, err := b.Sys.Fetch(ctx, url, maxKeyBytes)
			if err != nil {
				return err
			}
			if err := VerifyKeyFingerprint(data, fingerprint); err != nil {
				return err
			}
			if err := b.Sys.MkdirAll(parentDir(path), modePublicDir); err != nil {
				return err
			}
			if err := b.Sys.WriteFile(path, data, modeRootPublic); err != nil {
				return err
			}
			if after != nil {
				_, err := run(ctx, b.Sys, *after)
				return err
			}
			return nil
		},
		Undo: func(context.Context) error { return b.Sys.Remove(path) },
	}
	if after != nil {
		s.Commands = []string{after.String()}
	}
	return s
}

func parentDir(path string) string {
	i := strings.LastIndex(path, "/")
	if i <= 0 {
		return "/"
	}
	return path[:i]
}
