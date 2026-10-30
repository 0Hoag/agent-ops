package budget

import "time"

type Request struct {
	RepositoryID *string   `json:"repository_id"`
	Wave         string    `json:"wave"`
	TokenLimit   *int64    `json:"token_limit"`
	USDLimit     *float64  `json:"usd_limit"`
	StartsAt     time.Time `json:"starts_at"`
	EndsAt       time.Time `json:"ends_at"`
}
