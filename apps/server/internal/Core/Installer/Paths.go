package installer

import (
	_ "embed"

	config "github.com/Khaymah/dhow/apps/server/internal/Config"
)

const (
	ServiceName      = "dhow"
	ServiceUser      = "dhow"
	RequiredPostgres = 18
	DefaultListen    = "127.0.0.1:8080"
	DatabaseName     = "dhow"
	DatabaseRole     = "dhow"
)

const (
	OSReleasePath   = "/etc/os-release"
	BinaryPath      = "/usr/local/bin/dhow-server"
	ConfigDir       = "/etc/dhow"
	CredentialsDir  = ConfigDir + "/credentials"
	EnvFilePath     = ConfigDir + "/dhow.env"
	AnswersPath     = ConfigDir + "/answers.yaml"
	SecretKeyPath   = CredentialsDir + "/" + config.CredentialSecretKey
	DatabaseURLPath = CredentialsDir + "/" + config.CredentialDatabaseURL
	KeyringPath     = CredentialsDir + "/" + config.CredentialKeyring
	UnitPath        = "/etc/systemd/system/" + ServiceName + ".service"
	StateDir        = "/var/lib/dhow"
	ComposeDir      = ConfigDir + "/compose"
	ComposeFilePath = ComposeDir + "/compose.yaml"
	ComposeEnvPath  = ComposeDir + "/.env"
	ComposeSecrets  = ComposeDir + "/secrets"
	ComposeKeyring  = ComposeSecrets + "/" + config.CredentialKeyring
	PostgresSocket  = "/var/run/postgresql"
)

const (
	CaddyfilePath   = "/etc/caddy/Caddyfile"
	CaddySitePath   = "/etc/caddy/dhow.caddyfile"
	NginxSitePath   = "/etc/nginx/conf.d/dhow.conf"
	AcmeWebroot     = "/var/www/dhow-acme"
	CertbotHookPath = "/etc/letsencrypt/renewal-hooks/deploy/dhow-reload-nginx"
	LetsEncryptLive = "/etc/letsencrypt/live"
)

const (
	AptKeyPath     = "/etc/apt/keyrings/dhow-pgdg.asc"
	AptSourcePath  = "/etc/apt/sources.list.d/dhow-pgdg.sources"
	DnfKeyPath     = "/etc/pki/rpm-gpg/RPM-GPG-KEY-dhow-pgdg"
	DnfRepoPath    = "/etc/yum.repos.d/dhow-pgdg.repo"
	DnfExcludePath = "/etc/dnf/libdnf5.conf.d/90-dhow-pgdg.conf"
	AptKeyURL      = "https://www.postgresql.org/media/keys/ACCC4CF8.asc"
	DnfKeyURL      = "https://download.postgresql.org/pub/repos/yum/keys/PGDG-RPM-GPG-KEY-RHEL"
	DnfRepoBase    = "https://download.postgresql.org/pub/repos/yum/18/"
	AptKeyFinger   = "B97B0AFCAA1A47F044F244A07FCC7D46ACCC4CF8"
	DnfKeyFinger   = "D4BF08AE67A0B4C7A1DBCCD240BCA2B408B40D20"
	maxKeyBytes    = 64 << 10
	ComposeProject = "dhow"
	ComposeImage   = "ghcr.io/khaymah/dhow"
	ComposeEdgeNet = "dhow_edge"
	ContainerUID   = 65532
	ContainerUser  = "65532"
)

var (
	//go:embed Assets/dhow.service
	UnitFile []byte
	//go:embed Assets/compose.yaml
	ComposeFile []byte
	//go:embed Assets/proxy/nginx.conf
	NginxFixture []byte
	//go:embed Assets/proxy/Caddyfile
	CaddyFixture []byte
)

var (
	modeRootPrivate = FileMode{Perm: 0o600}
	modeRootPublic  = FileMode{Perm: 0o644}
	modeRootExec    = FileMode{Perm: 0o755}
	modeRootDir     = FileMode{Perm: 0o700}
	modePublicDir   = FileMode{Perm: 0o755}
	modeContainer   = FileMode{Perm: 0o600, Owner: ContainerUser, Group: ContainerUser}
)
