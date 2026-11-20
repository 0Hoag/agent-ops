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
	expectAcquisition(mock, "org-a", sqlmock.AnyArg())
	mock.ExpectCommit()
	occurrenceSQL := `INSERT INTO alert_occurrences`
	deliverySQL := regexp.QuoteMeta("ON CONFLICT (occurrence_id, channel) DO UPDATE SET")
	mock.ExpectExec(occurrenceSQL).WithArgs(sqlmock.AnyArg(), "org-a", sqlmock.AnyArg(), "repo", RuleLowCache, "worker", "cache hit below threshold", now, "org-a", sqlmock.AnyArg()).WillReturnResult(sqlmockResult(1))
	mock.ExpectExec(`INSERT INTO alert_incident_states`).WithArgs("org-a", sqlmock.AnyArg(), false).WillReturnResult(sqlmockResult(1))
	mock.ExpectExec(`INSERT INTO alert_incident_states`).WithArgs("org-a", sqlmock.AnyArg()).WillReturnResult(sqlmockResult(1))
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

func TestEvaluateAndDeliverRetriesFailedDeliveryWithoutRepeatingSuccess(t *testing.T) {
	db, mock := alertDB(t)
	mock.MatchExpectationsInOrder(false)
	repo := NewRepository(db)
	now := time.Now()
	expectAcquisition(mock, "org-a", sqlmock.AnyArg())
	mock.ExpectCommit()
	expectAcquisition(mock, "org-a", sqlmock.AnyArg())
	mock.ExpectCommit()
	occurrenceSQL := `INSERT INTO alert_occurrences`
	deliverySQL := regexp.QuoteMeta("ON CONFLICT (occurrence_id, channel) DO UPDATE SET")
	mock.ExpectExec(occurrenceSQL).WithArgs(sqlmock.AnyArg(), "org-a", sqlmock.AnyArg(), "repo", RuleLowCache, "worker", "cache hit below threshold", now, "org-a", sqlmock.AnyArg()).WillReturnResult(sqlmockResult(1))
	mock.ExpectExec(`INSERT INTO alert_incident_states`).WithArgs("org-a", sqlmock.AnyArg(), false).WillReturnResult(sqlmockResult(1))
	mock.ExpectExec(`INSERT INTO alert_incident_states`).WithArgs("org-a", sqlmock.AnyArg(), false).WillReturnResult(sqlmockResult(1))
	mock.ExpectExec(`INSERT INTO alert_incident_states`).WithArgs("org-a", sqlmock.AnyArg()).WillReturnResult(sqlmockResult(1))
	mock.ExpectExec(deliverySQL).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "email", false, "down", now).WillReturnResult(sqlmockResult(1))
	mock.ExpectExec(occurrenceSQL).WithArgs(sqlmock.AnyArg(), "org-a", sqlmock.AnyArg(), "repo", RuleLowCache, "worker", "cache hit below threshold", now, "org-a", sqlmock.AnyArg()).WillReturnResult(sqlmockResult(0))
	mock.ExpectQuery(`SELECT .* FROM "alert_occurrences" WHERE organization_id = \$1 AND alert_key = \$2 ORDER BY created_at DESC, id DESC LIMIT \$3`).
		WithArgs("org-a", sqlmock.AnyArg(), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "organization_id", "alert_key", "repository", "rule", "subject", "message", "detected_at"}).
			AddRow("persisted-occurrence", "org-a", "key", "repo", RuleLowCache, "worker", "cache hit below threshold", now))
	mock.ExpectQuery(`SELECT .*"success".*FROM "alert_deliveries".*WHERE occurrence_id = \$1 AND channel = \$2.*`).
		WithArgs(sqlmock.AnyArg(), "email", sqlmock.AnyArg()).WillReturnRows(sqlmock.NewRows([]string{"success"}).AddRow(false))
	mock.ExpectExec(deliverySQL).WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), "email", true, "", now).WillReturnResult(sqlmockResult(1))
	notifier := &fakeNotifier{err: errors.New("down")}
	svc := NewService(NewEvaluator(Thresholds{MinCacheHit: 0.9}), repo, map[string]notification.Notifier{"email": notifier})
	obs := Observation{OrganizationID: "org-a", Repository: "repo", Runs: []RunObservation{{Worker: "worker", InputTokens: 100, CachedInputTokens: 0}}}
	require.NoError(t, svc.EvaluateAndDeliver(context.Background(), now, obs))
	notifier.err = nil
	require.NoError(t, svc.EvaluateAndDeliver(context.Background(), now, obs))
	require.Equal(t, 2, notifier.calls)
	require.NoError(t, mock.ExpectationsWereMet())
}

func sqlmockResult(rows int64) driver.Result { return sqlmock.NewResult(0, rows) }
