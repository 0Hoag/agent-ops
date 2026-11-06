package alert

import (
	"context"
	"time"

	"github.com/0Hoag/agent-ops/api/pkg/handlers/notification"
)

type Service struct {
	evaluator  *Evaluator
	repository *Repository
	channels   map[string]notification.Notifier
}

func NewService(e *Evaluator, r *Repository, channels map[string]notification.Notifier) *Service {
	return &Service{evaluator: e, repository: r, channels: channels}
}
func (s *Service) EvaluateAndDeliver(ctx context.Context, now time.Time, observation Observation) error {
	for _, a := range s.evaluator.Evaluate(ctx, now, observation) {
		row, inserted, err := s.repository.Record(ctx, a)
		if err != nil {
			return err
		}
		if !inserted {
			continue
		}
		for channel, sender := range s.channels {
			err := sender.Notify(ctx, notification.Message{Subject: a.Rule, Body: a.Message})
			d := Delivery{OccurrenceID: row.ID, Channel: channel, Success: err == nil, DeliveredAt: now}
			if err != nil {
				d.Error = err.Error()
			}
			if saveErr := s.repository.RecordDelivery(ctx, d); saveErr != nil {
				return saveErr
			}
		}
	}
	return nil
}
