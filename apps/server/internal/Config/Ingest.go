package config

import "time"

const (
	IngestPrefix              = "/ingest/v1"
	IngestArtifactsPath       = "/artifacts/{sha256}"
	IngestTokenPrefix         = "dhow_ing_"
	IngestTokenRandomBytes    = 32
	IngestTokenDisplayChars   = 8
	IngestTokenDefaultDays    = 90
	IngestTokenMaxDays        = 365
	IngestTokenNameMax        = 64
	IngestMaxPatterns         = 20
	IngestMaxTokensPerProject = 50
	IngestPatternMax          = 200
	IngestProvenanceMax       = 4 << 10
	IngestProvenanceFields    = 16
	IngestProvenanceValue     = 256
	IngestExpandFactor        = 8
	IngestConcurrent          = 8
	IngestBusyRetry           = 30 * time.Second
	IngestPerTokenMax         = 30
	IngestPerTokenWindow      = time.Hour
	IngestPerIPMax            = 120
	IngestPerIPWindow         = time.Minute
	IngestRejectAuditEvery    = time.Minute
	IngestLastUsedEvery       = time.Minute
	IngestIdleTimeout         = time.Minute
	IngestMinRate             = 256 << 10
	IngestRateGrace           = 15 * time.Second
	IngestDeadlineFloor       = 2 * time.Minute
	IngestDeadlineCap         = 30 * time.Minute
	IngestResponseGrace       = 30 * time.Second
	IngestPerTokenInflight    = 1
	IngestOrgShareDivisor     = 4
	IngestPerOrgInflight      = IngestConcurrent / IngestOrgShareDivisor
	IngestPerNetworkInflight  = 2
	IngestTokenMaxBytes       = 10 << 30
	IngestListMax             = 100
	IngestPinMax              = 90 * 24 * time.Hour
	IngestHeaderName          = "Dhow-Ingest-Name"
	IngestHeaderRef           = "Dhow-Ingest-Ref"
	IngestHeaderRefKind       = "Dhow-Ingest-Ref-Kind"
	IngestHeaderSha           = "Dhow-Ingest-Sha"
	IngestHeaderTrigger       = "Dhow-Ingest-Trigger"
	IngestHeaderProvenance    = "Dhow-Ingest-Provenance"
)
