// Package transcript parses the local JSONL usage records emitted by supported
// worker harnesses. It intentionally returns metadata and counters only.
package transcript

import "time"

type Usage struct {
	Input, CachedInput, CacheWrite, Output, Reasoning int64
}

type Session struct {
	ID             string
	Model          string
	Started, Ended time.Time
	Usage          Usage
	Steps          []Step
}

// Step contains only usage and safe tool metadata; transcript content is never retained.
type Step struct {
	Started, Ended time.Time
	Usage          Usage
	ToolName       string
	CacheHit       bool
}
