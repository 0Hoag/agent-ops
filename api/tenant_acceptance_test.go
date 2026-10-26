package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/board"
	"github.com/0Hoag/agent-ops/api/pkg/config"
	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const tenantDSNEnv = "W7M_TENANT_ACCEPTANCE_DSN"
const orgA = "d7000000-0000-4000-8000-000000000001"
const orgB = "d7000000-0000-4000-8000-000000000002"
const repoA = "d7000000-0000-4000-8000-000000000011"
const repoB = "d7000000-0000-4000-8000-000000000012"
const accountA = "d7000000-0000-4000-8000-000000000021"
const accountB = "d7000000-0000-4000-8000-000000000022"

func TestW7MTenantAcceptance(t *testing.T) {
	dsn := os.Getenv(tenantDSNEnv)
	if dsn == "" {
		t.Skip("set " + tenantDSNEnv + " to run disposable PostgreSQL acceptance")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sql, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sql.Close() })
	require.NoError(t, database.RunMigrations(context.Background(), db))
	seedTenantFixture(t, db)
	hub := board.NewHub()
	r := httptest.NewServer(buildAcceptanceRouter(t, db, hub))
	t.Cleanup(r.Close)
	tokenA := tenantToken(t, accountA, "a@example.invalid", orgA)
	tokenB := tenantToken(t, accountB, "b@example.invalid", orgB)

	assertJSON(t, r.URL+"/api/v1/repositories", tokenA, http.StatusOK, `"total":1`, `tenant/a`)
	assertJSON(t, r.URL+"/api/v1/repositories", tokenB, http.StatusOK, `"total":1`, `tenant/b`)
	assertJSON(t, r.URL+"/api/v1/board?repository_id="+repoA, tokenA, http.StatusOK, `tenant-a issue`)
	assertNotJSON(t, r.URL+"/api/v1/board?repository_id="+repoA, tokenA, `tenant-b issue`, `tenant-b pr`, `b-run`)
	assertJSON(t, r.URL+"/api/v1/board?repository_id="+repoB, tokenA, http.StatusNotFound, `repository not found`)
	assertJSON(t, r.URL+"/api/v1/insights?repository_id="+repoB, tokenA, http.StatusNotFound, `repository not found`)
	assertJSON(t, r.URL+"/api/v1/agent-runs?repository_id="+repoB, tokenA, http.StatusOK, `"data":[]`)
	assertJSON(t, r.URL+"/api/v1/agent-runs/aggregate?repository_id="+repoB, tokenA, http.StatusOK, `"data":[]`)
	assertJSON(t, r.URL+"/api/v1/agent-runs?issue_number=7002", tokenA, http.StatusOK, `"data":[]`)
	assertJSON(t, r.URL+"/api/v1/agent-runs", tokenA, http.StatusOK, `"repository_id":"`+repoA+`"`)
	assertNotJSON(t, r.URL+"/api/v1/agent-runs", tokenA, `"repository_id":"`+repoB+`"`)
	assertJSON(t, r.URL+"/api/v1/agent-runs/aggregate", tokenA, http.StatusOK, `"runs":1`, `"cost_usd":0.000002`)
	assertJSON(t, r.URL+"/api/v1/agent-runs", tokenB, http.StatusOK, `"repository_id":"`+repoB+`"`)
	assertNotJSON(t, r.URL+"/api/v1/agent-runs", tokenB, `"repository_id":"`+repoA+`"`)
	assertNotJSON(t, r.URL+"/api/v1/agent-runs/aggregate", tokenB, `"issue_number":7001`)

	postAgentRun(t, r.URL+"/api/v1/agent-runs", "machine", tokenA, repoA, http.StatusCreated)
	postAgentRun(t, r.URL+"/api/v1/agent-runs", "machine", tokenA, repoB, http.StatusUnauthorized)

	wsA := dialTenantWS(t, r.URL+"/api/v1/board/ws?repository_id="+repoA, tokenA)
	t.Cleanup(func() { _ = wsA.Close() })
	wsB := dialTenantWS(t, r.URL+"/api/v1/board/ws?repository_id="+repoB, tokenB)
	t.Cleanup(func() { _ = wsB.Close() })
	require.Eventually(t, func() bool { return hub.SubscriberCount(repoA) == 1 && hub.SubscriberCount(repoB) == 1 }, time.Second, 10*time.Millisecond)
	foreign, foreignResp, err := websocket.DefaultDialer.Dial(strings.Replace(r.URL, "http", "ws", 1)+"/api/v1/board/ws?repository_id="+repoB, http.Header{"Authorization": []string{"Bearer " + tokenA}})
	if foreignResp != nil {
		defer func() { _ = foreignResp.Body.Close() }()
	}
	if foreign != nil {
		defer func() { _ = foreign.Close() }()
	}
	require.Error(t, err, "cross-tenant websocket admission must be rejected")
	require.NoError(t, wsA.SetReadDeadline(time.Now().Add(100*time.Millisecond)))
	hub.Publish(repoB)
	_, _, err = wsA.ReadMessage()
	require.Error(t, err, "cross-tenant websocket event leaked")
	_ = wsA.Close()
	wsA = dialTenantWS(t, r.URL+"/api/v1/board/ws?repository_id="+repoA, tokenA)
	t.Cleanup(func() { _ = wsA.Close() })
	require.Eventually(t, func() bool { return hub.SubscriberCount(repoA) == 1 }, time.Second, 10*time.Millisecond)
	require.NoError(t, wsA.SetReadDeadline(time.Now().Add(time.Second)))
	hub.Publish(repoA)
	_, msg, err := wsA.ReadMessage()
	require.NoError(t, err)
	var event map[string]string
	require.NoError(t, json.Unmarshal(msg, &event))
	require.Equal(t, repoA, event["repository_id"])

	require.NoError(t, db.Exec("DELETE FROM organization_memberships WHERE organization_id = ? AND account_id = ?", orgA, accountA).Error)
	assertJSON(t, r.URL+"/api/v1/repositories", tokenA, http.StatusUnauthorized, `organization membership required`)
}

func seedTenantFixture(t *testing.T, db *gorm.DB) {
	for _, table := range []string{"agent_runs", "pipeline_runs", "pull_requests", "issues", "repositories", "organization_memberships", "organizations", "accounts"} {
		require.NoError(t, db.Exec("DELETE FROM "+table).Error)
	}
	q := `INSERT INTO accounts (id,email,password_hash) VALUES
('d7000000-0000-4000-8000-000000000021','a@example.invalid','x'),('d7000000-0000-4000-8000-000000000022','b@example.invalid','x');
INSERT INTO organizations (id,name) VALUES ('d7000000-0000-4000-8000-000000000001','Tenant A'),('d7000000-0000-4000-8000-000000000002','Tenant B');
INSERT INTO organization_memberships (id,organization_id,account_id,role) VALUES
('d7000000-0000-4000-8000-000000000031','d7000000-0000-4000-8000-000000000001','d7000000-0000-4000-8000-000000000021','owner'),
('d7000000-0000-4000-8000-000000000032','d7000000-0000-4000-8000-000000000002','d7000000-0000-4000-8000-000000000022','owner');
INSERT INTO repositories (id,provider,full_name,webhook_secret_env_ref,organization_id) VALUES
('d7000000-0000-4000-8000-000000000011','github','tenant/a','W7M_A','d7000000-0000-4000-8000-000000000001'),
('d7000000-0000-4000-8000-000000000012','github','tenant/b','W7M_B','d7000000-0000-4000-8000-000000000002');
INSERT INTO issues (id,repository_id,number,title,state,status,labels) VALUES
('d7000000-0000-4000-8000-000000000041','d7000000-0000-4000-8000-000000000011',7001,'tenant-a issue','open','todo','[]'),
('d7000000-0000-4000-8000-000000000042','d7000000-0000-4000-8000-000000000012',7002,'tenant-b issue','open','todo','[]');
INSERT INTO pull_requests (id,repository_id,number,title,state,head_sha,branch,closes) VALUES
('d7000000-0000-4000-8000-000000000051','d7000000-0000-4000-8000-000000000011',71,'tenant-a pr','open','a-sha','a','[7001]'),
('d7000000-0000-4000-8000-000000000052','d7000000-0000-4000-8000-000000000012',72,'tenant-b pr','open','b-sha','b','[7002]');
INSERT INTO pipeline_runs (id,repository_id,external_id,pr_number,sha,status,conclusion) VALUES
('d7000000-0000-4000-8000-000000000061','d7000000-0000-4000-8000-000000000011','a-run',71,'a-sha','completed','success'),
('d7000000-0000-4000-8000-000000000062','d7000000-0000-4000-8000-000000000012','b-run',72,'b-sha','completed','success');
INSERT INTO agent_runs (id,repository_id,wave,worker,model,issue_numbers,input_tokens,cached_input_tokens,cache_write_tokens,output_tokens,reasoning_tokens,started_at,ended_at) VALUES
('d7000000-0000-4000-8000-000000000071','d7000000-0000-4000-8000-000000000011','W7','a','known','[7001]',1,0,0,1,0,now(),now()),
('d7000000-0000-4000-8000-000000000072','d7000000-0000-4000-8000-000000000012','W7','b','known','[7002]',2,0,0,2,0,now(),now());`
	require.NoError(t, db.Exec(q).Error)
}

func buildAcceptanceRouter(t *testing.T, db *gorm.DB, hub *board.Hub) http.Handler {
	r := gin.New()
	registerRoutesWithHub(r, database.AsPinger(db), db, config.Config{JWTSecret: "w7m-secret", JWTExpiration: time.Hour, AgentRunToken: "machine", AgentRunModelPrices: map[string]json.RawMessage{"known": json.RawMessage(`{"input_per_million":1,"output_per_million":1}`)}, Repositories: []config.RepositoryConfig{{Provider: "github", FullName: "tenant/a"}}}, hub)
	return r
}

func tenantToken(t *testing.T, account, email, organization string) string {
	now := time.Now()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": account, "email": email, "organization_id": organization, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()}).SignedString([]byte("w7m-secret"))
	require.NoError(t, err)
	return token
}

func assertJSON(t *testing.T, url, token string, status int, contains ...string) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	require.Equal(t, status, res.StatusCode, string(body))
	for _, want := range contains {
		require.Contains(t, string(body), want)
	}
}

func assertNotJSON(t *testing.T, url, token string, absent ...string) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	body, err := io.ReadAll(res.Body)
	require.NoError(t, err)
	for _, value := range absent {
		require.NotContains(t, string(body), value)
	}
}

func postAgentRun(t *testing.T, url, machineToken, bearerToken, repositoryID string, status int) {
	body := `{"repository_id":"` + repositoryID + `","wave":"W7","worker":"acceptance","model":"known","issue_numbers":[7001],"input_tokens":1,"started_at":"2026-01-01T00:00:00Z","ended_at":"2026-01-01T00:00:01Z"}`
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Run-Token", machineToken)
	req.Header.Set("Authorization", "Bearer "+bearerToken)
	res, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer func() { _ = res.Body.Close() }()
	got, _ := io.ReadAll(res.Body)
	require.Equal(t, status, res.StatusCode, string(got))
}

func dialTenantWS(t *testing.T, url, token string) *websocket.Conn {
	url = "ws" + url[len("http"):]
	h := http.Header{"Authorization": []string{"Bearer " + token}}
	conn, res, err := websocket.DefaultDialer.Dial(url, h)
	if res != nil {
		defer func() { _ = res.Body.Close() }()
	}
	require.NoError(t, err)
	return conn
}
