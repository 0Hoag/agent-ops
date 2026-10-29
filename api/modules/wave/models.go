package wave

import (
	"errors"
	"time"
)

const (
	Todo       = "todo"
	InProgress = "in_progress"
	Review     = "review"
	Blocked    = "blocked"
	Done       = "done"
)

var ErrNotFound = errors.New("wave not found")
var ErrForbidden = errors.New("wave write forbidden")
var ErrInvalidState = errors.New("invalid wave state")
var ErrInvalidTransition = errors.New("invalid wave state transition")

type Wave struct {
	ID             string    `json:"id" gorm:"type:uuid;primaryKey"`
	OrganizationID string    `json:"organization_id" gorm:"type:uuid;not null"`
	Name           string    `json:"name"`
	Brief          string    `json:"brief"`
	State          string    `json:"state"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (Wave) TableName() string { return "waves" }

func validState(s string) bool {
	return s == Todo || s == InProgress || s == Review || s == Blocked || s == Done
}

func validTransition(from, to string) bool {
	if from == to {
		return true
	}
	allowed := map[string]map[string]bool{
		Todo:       {InProgress: true, Blocked: true},
		InProgress: {Review: true, Blocked: true, Todo: true},
		Review:     {Done: true, InProgress: true, Blocked: true},
		Blocked:    {Todo: true, InProgress: true},
		Done:       {Review: true},
	}
	return allowed[from][to]
}
