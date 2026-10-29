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

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
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
