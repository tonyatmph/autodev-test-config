package runner

import (
)

// TelemetryEvent represents a discrete unit of activity from a Cell.
type TelemetryEvent struct {
	Type    string         // "log", "metric", "fitness_update"
	Payload map[string]any
}

// TelemetryStream is the pipe that transports activity back to the orchestrator.
type TelemetryStream chan TelemetryEvent
