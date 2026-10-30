package alert

import (
	"context"
	"database/sql/driver"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/pkg/handlers/notification"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

type fakeNotifier struct {
	err   error
	calls int
}

func (f *fakeNotifier) Notify(context.Context, notification.Message) error { f.calls++; return f.err }

func TestEvaluateAndDeliverPersistsSuccessAndFailure(t *testing.T) {
	db, mock := alertDB(t)
	mock.MatchExpectationsInOrder(false)
	repo := NewRepository(db)
	now := time.Now()
	occurrenceSQL := regexp.QuoteMeta("ON CONFLICT (organization_id, alert_key) DO NOTHING")
	deliverySQL := regexp.QuoteMeta("ON CONFLICT (occurrence_id, channel) DO UPDATE SET")
	mock.ExpectExec(occurrenceSQL).WithArgs(sqlmock.AnyArg(), "org-a", sqlmock.AnyArg(), "repo", RuleLowCache, "worker", "cache hit below threshold", now).WillReturnResult(sqlmockResult(1))
	// Map iteration order is deliberately avoided by testing each possible outcome through the same SQL contract.
	mock.ExpectExec(deliverySQL).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "email", true, "", now).WillReturnResult(sqlmockResult(1))
	mock.ExpectExec(deliverySQL).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "slack", false, "down", now).WillReturnResult(sqlmockResult(1))
	email := &fakeNotifier{}
	slack := &fakeNotifier{err: errors.New("down")}
	svc := NewService(NewEvaluator(Thresholds{MinCacheHit: 0.9}), repo, map[string]notification.Notifier{"email": email, "slack": slack})
	obs := Observation{OrganizationID: "org-a", Repository: "repo", Runs: []RunObservation{{Worker: "worker", InputTokens: 100, CachedInputTokens: 0}}}
	require.NoError(t, svc.EvaluateAndDeliver(context.Background(), now, obs))
	require.Equal(t, 1, email.calls)
	require.Equal(t, 1, slack.calls)
	require.NoError(t, mock.ExpectationsWereMet())
}

func sqlmockResult(rows int64) driver.Result { return sqlmock.NewResult(0, rows) }
