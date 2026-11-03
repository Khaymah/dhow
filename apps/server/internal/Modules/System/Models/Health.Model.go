package models

type HealthState string

const (
	HealthOK          HealthState = "ok"
	HealthUnavailable HealthState = "unavailable"
)

type HealthCheckName string

const (
	CheckDatabase      HealthCheckName = "database"
	CheckMigrations    HealthCheckName = "migrations"
	CheckLogPartitions HealthCheckName = "log_partitions"
	CheckSettings      HealthCheckName = "settings"
	CheckKeyring       HealthCheckName = "keyring"
)

type HealthCheck struct {
	Name HealthCheckName
	Ok   bool
}

type HealthStatus struct {
	Status HealthState
	Checks []HealthCheck
}

func (h HealthStatus) Ready() bool { return h.Status == HealthOK }
