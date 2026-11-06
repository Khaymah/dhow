package config

import "time"

const (
	PipelineBatch                 = 100
	PipelineClaim                 = time.Minute
	PipelineStartInterval         = time.Second
	PipelineReconcileInterval     = 5 * time.Second
	PipelineDriftGrace            = 5 * time.Second
	PipelineGateInterval          = 5 * time.Second
	PipelineDefinitionTimeout     = 2 * time.Minute
	PipelinePinBudget             = 30 * time.Second
	PipelineFetchTimeout          = 30 * time.Second
	PipelineTriggerPerUser        = 60
	PipelineTriggerPerProject     = 240
	PipelineTriggerWindow         = 10 * time.Minute
	PipelineCheckPerUser          = 10
	PipelineCheckPerProject       = 30
	PipelineWebhookRunsPerProject = 600
	PipelineWebhookRunsWindow     = time.Hour
)
