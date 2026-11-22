package model

// AdminBatchResult reports validated input and completed writes independently.
// A failed operation may have durable pending work which is retried safely.
type AdminBatchResult struct {
	Validated int  `json:"validated"`
	Written   int  `json:"written"`
	Skipped   int  `json:"skipped"`
	DryRun    bool `json:"dry_run"`
}
