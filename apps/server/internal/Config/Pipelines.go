package config

import "time"

const (
	PipelineBatch             = 100
	PipelineClaim             = time.Minute
	PipelineStartInterval     = time.Second
	PipelineReconcileInterval = 5 * time.Second
	PipelineDriftGrace        = 5 * time.Second
	PipelineDefinitionTimeout = 2 * time.Minute
)
