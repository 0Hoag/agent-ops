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
	if err := ctx.Err(); err != nil {
		return err
	}
	for _, a := range s.evaluator.Evaluate(ctx, now, observation) {
		row, inserted, err := s.repository.Record(ctx, a)
		if err != nil {
			return err
		}
		eligible := s.channels
		if !inserted {
			eligible = make(map[string]notification.Notifier, len(s.channels))
			for channel, sender := range s.channels {
				delivery, found, err := s.repository.GetDelivery(ctx, row.ID, channel)
				if err != nil {
					return err
				}
				if !found || !delivery.Success {
					eligible[channel] = sender
				}
			}
		}
		for channel, sender := range eligible {
			if err := ctx.Err(); err != nil {
				return err
			}
			err := sender.Notify(ctx, notification.Message{Subject: a.Rule, Body: a.Message})
			if err != nil {
				s.evaluator.AllowRetry(a.Key)
			}
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
