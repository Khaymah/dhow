package config

import "os"

const (
	EnvTestDatabaseURL = "DHOW_TEST_DATABASE_URL"
	EnvTestPgDump      = "DHOW_TEST_PG_DUMP"
	envCI              = "CI"
	defaultPgDump      = "pg_dump"
)

func TestDatabaseURL() string { return os.Getenv(EnvTestDatabaseURL) }

func TestPgDump() string {
	if v := os.Getenv(EnvTestPgDump); v != "" {
		return v
	}
	return defaultPgDump
}

func RunningInCI() bool { return os.Getenv(envCI) != "" }
