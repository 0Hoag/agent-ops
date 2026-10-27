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
}
