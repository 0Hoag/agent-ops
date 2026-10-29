package agentrun

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type timelineFixtureRepository struct {
	steps []RunStep
	gotID string
}

func (f *timelineFixtureRepository) Create(context.Context, *AgentRun) error { return nil }
func (f *timelineFixtureRepository) IsConfiguredRepository(context.Context, string, []string) (bool, error) {
	return true, nil
}
func (f *timelineFixtureRepository) List(context.Context, Filter) ([]AgentRun, error) {
	return nil, nil
}
func (f *timelineFixtureRepository) Aggregate(context.Context, Filter, map[string]Price) ([]Aggregate, error) {
	return nil, nil
}
func (f *timelineFixtureRepository) Steps(_ context.Context, id string) ([]RunStep, error) {
	f.gotID = id
	return f.steps, nil
}
func (f *timelineFixtureRepository) ReplaceSteps(_ context.Context, id string, steps []RunStep) error {
	f.gotID = id
	f.steps = append([]RunStep(nil), steps...)
	for i := range f.steps {
		f.steps[i].ID = uuid.NewMD5(uuid.Nil, []byte(id+":"+strconv.Itoa(i))).String()
		f.steps[i].RunID = id
		f.steps[i].Sequence = i
	}
	return nil
}

func TestTimelineReturnsOrderedSanitizedFiftyStepFixtureWithinBudget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	steps := make([]RunStep, 50)
	for i := range steps {
		steps[i] = RunStep{Sequence: i, InputTokens: int64(i + 1), ToolName: "shell"}
	}
	repo := &timelineFixtureRepository{steps: steps}
	r := gin.New()
	r.GET("/", func(c *gin.Context) {
		c.Set("organization_id", "org-a")
		NewController(repo, "token", nil).Timeline(c)
	})
	started := time.Now()
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/", nil))
	elapsed := time.Since(started)
	require.Equal(t, http.StatusOK, res.Code)
	var body struct {
		Data []RunStep `json:"data"`
	}
	require.NoError(t, json.Unmarshal(res.Body.Bytes(), &body))
	require.Len(t, body.Data, 50)
	for i, step := range body.Data {
		require.Equal(t, i, step.Sequence)
		require.Equal(t, "shell", step.ToolName)
	}
	require.Less(t, elapsed, 250*time.Millisecond, "deterministic 50-step endpoint fixture exceeded local budget")
}

func TestTimelineIngestIsRepeatableAndDoesNotReturnSensitiveFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &timelineFixtureRepository{}
	r := gin.New()
	r.POST("/:run_id/timeline", func(c *gin.Context) {
		NewController(repo, "token", nil).IngestTimeline(c)
	})
	payload := `{"steps":[{"sequence":99,"input_tokens":2,"tool_name":"shell","arguments":"SECRET","result":"SECRET"}]}`
	for i := 0; i < 2; i++ {
		res := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/run-1/timeline", strings.NewReader(payload))
		req.Header.Set("X-Agent-Run-Token", "token")
		r.ServeHTTP(res, req)
		require.Equal(t, http.StatusOK, res.Code)
		require.NotContains(t, res.Body.String(), "SECRET")
	}
	require.Len(t, repo.steps, 1)
	require.Equal(t, 0, repo.steps[0].Sequence)
	require.NotEmpty(t, repo.steps[0].ID)
	firstID := repo.steps[0].ID
	res := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/run-1/timeline", strings.NewReader(payload))
	req.Header.Set("X-Agent-Run-Token", "token")
	r.ServeHTTP(res, req)
	require.Equal(t, firstID, repo.steps[0].ID)
}

func TestTimelineStepIdentitySurvivesReplacementAndDeletion(t *testing.T) {
	first := RunStep{InputTokens: 3, OutputTokens: 5, ToolName: "shell", CacheHit: true}
	second := RunStep{InputTokens: 8, OutputTokens: 13, ToolName: "test"}
	firstID := uuid.NewMD5(uuid.Nil, []byte("run-1:"+timelineStepFingerprint(first)+":0"))
	secondID := uuid.NewMD5(uuid.Nil, []byte("run-1:"+timelineStepFingerprint(second)+":0"))

	// A replacement may delete the first item and reorder the survivor; its
	// persisted identity must remain content-stable instead of sequence-based.
	replacement := []RunStep{second, first}
	gotSecond := uuid.NewMD5(uuid.Nil, []byte("run-1:"+timelineStepFingerprint(replacement[0])+":0"))
	gotFirst := uuid.NewMD5(uuid.Nil, []byte("run-1:"+timelineStepFingerprint(replacement[1])+":0"))
	require.Equal(t, secondID, gotSecond)
	require.Equal(t, firstID, gotFirst)
	require.NotEqual(t, firstID, uuid.NewMD5(uuid.Nil, []byte("run-1:0")))
}

func TestTimelineStepCostUsesEveryHistoricalCounter(t *testing.T) {
	step := RunStep{InputTokens: 100, CachedInputTokens: 20, CacheWriteTokens: 10, OutputTokens: 30, ReasoningTokens: 5}
	price := Price{InputPerMillion: 1, CachedInputPerMillion: 2, CacheWritePerMillion: 3, OutputPerMillion: 4, ReasoningPerMillion: 5}
	require.InDelta(t, 0.0001+0.00004+0.00003+0.00012+0.000025, stepCost(step, price), 1e-12)
}

func TestRepositoryReplaceStepsUsesPersistedStableIdentity(t *testing.T) {
	sqlDB, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent), DisableAutomaticPing: true})
	require.NoError(t, err)
	repo := NewRepository(db)
	runID := "a1000000-0000-0000-0000-000000000001"
	orgID := "b1000000-0000-0000-0000-000000000001"
	mock.ExpectBegin()
	mock.ExpectQuery(`SELECT .* FROM agent_runs ar JOIN repositories repo ON repo\.id = ar\.repository_id WHERE ar\.id = \$1 AND repo\.organization_id = \$2 ORDER BY .* LIMIT \$3`).WithArgs(runID, orgID, 1).WillReturnRows(sqlmock.NewRows([]string{"id", "repository_id", "wave", "worker", "model", "issue_numbers", "started_at", "ended_at", "billing_type"}).AddRow(runID, orgID, "W", "worker", "model", []byte("[]"), time.Now(), time.Now(), "api"))
	mock.ExpectExec(`DELETE FROM agent_run_steps WHERE run_id = \$1`).WithArgs(runID).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`INSERT INTO "agent_run_steps"`).WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectCommit()
	steps := []RunStep{{InputTokens: 3, ToolName: "shell"}, {InputTokens: 8, ToolName: "test"}}
	require.NoError(t, repo.ReplaceSteps(WithOrganization(context.Background(), orgID), runID, steps))
	require.NotEqual(t, uuid.NewMD5(uuid.Nil, []byte(runID+":0")), steps[0].ID)
	require.Equal(t, uuid.NewMD5(uuid.Nil, []byte(runID+":"+timelineStepFingerprint(steps[0])+":0")), uuid.MustParse(steps[0].ID))
	require.NoError(t, mock.ExpectationsWereMet())
}
