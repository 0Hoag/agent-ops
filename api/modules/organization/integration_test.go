package organization

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/0Hoag/agent-ops/api/modules/auth"
	"github.com/0Hoag/agent-ops/api/pkg/database"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ORGANIZATION_TEST_DATABASE_URL must name an owned disposable database.
// Each test additionally isolates its records in a unique schema.
func integrationDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("ORGANIZATION_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set ORGANIZATION_TEST_DATABASE_URL for real PostgreSQL acceptance")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	schema := "organization_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	require.NoError(t, db.Exec("CREATE SCHEMA "+schema).Error)
	require.NoError(t, db.Exec("SET search_path TO "+schema).Error)
	t.Cleanup(func() {
		require.NoError(t, db.Exec("DROP SCHEMA "+schema+" CASCADE").Error)
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, database.RunMigrations(context.Background(), db))
	return db
}

func testAccount(t *testing.T, db *gorm.DB) string {
	return testAccountWithEmail(t, db, uuid.NewString()+"@example.test")
}

func testAccountWithEmail(t *testing.T, db *gorm.DB, email string) string {
	t.Helper()
	id := uuid.NewString()
	require.NoError(t, db.Exec("INSERT INTO accounts (id,email,password_hash) VALUES (?,?,?)", id, email, "fixture-secret").Error)
	return id
}

func testRouter(db *gorm.DB) http.Handler {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	NewRoutes(NewController(NewRepository(db), auth.NewService(nil, "organization-test-secret", time.Hour))).Register(r.Group("/api/v1"))
	return r
}

func request(t *testing.T, router http.Handler, actor, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	urlPath := "/api/v1/organizations" + path
	if strings.HasPrefix(path, "/../") {
		urlPath = "/api/v1/" + strings.TrimPrefix(path, "/../")
	}
	req := httptest.NewRequest(method, urlPath, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if actor != "" {
		token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": actor, "email": actor + "@example.test", "exp": time.Now().Add(time.Hour).Unix()}).SignedString([]byte("organization-test-secret"))
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestOrganizationHTTPIdentityAndIsolation(t *testing.T) {
	db := integrationDB(t)
	router := testRouter(db)
	owner := testAccountWithEmail(t, db, "owner@example.test")
	member := testAccountWithEmail(t, db, "member@example.test")
	outsider := testAccountWithEmail(t, db, "outsider@example.test")
	w := request(t, router, owner, "POST", "", `{"name":"Owned","account_id":"`+outsider+`"}`)
	require.Equal(t, 201, w.Code, w.Body.String())
	var result struct {
		Data Organization `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	id := result.Data.ID
	require.NotEmpty(t, id)
	m, err := NewRepository(db).Membership(context.Background(), id, owner)
	require.NoError(t, err)
	require.Equal(t, RoleOwner, m.Role)
	require.Equal(t, 204, request(t, router, owner, "POST", "/"+id+"/members", `{"account_id":"`+member+`","role":"member"}`).Code)
	membersResponse := request(t, router, owner, "GET", "/"+id+"/members", "")
	require.Equal(t, 200, membersResponse.Code, membersResponse.Body.String())
	var members struct {
		Data []Member `json:"data"`
	}
	require.NoError(t, json.Unmarshal(membersResponse.Body.Bytes(), &members))
	require.Len(t, members.Data, 2)
	require.Equal(t, owner, members.Data[0].AccountID)
	require.Equal(t, "owner@example.test", members.Data[0].Email)
	require.Equal(t, member, members.Data[1].AccountID)
	require.Equal(t, "member@example.test", members.Data[1].Email)
	require.NotContains(t, membersResponse.Body.String(), "password_hash")
	require.NotContains(t, membersResponse.Body.String(), "fixture-secret")
	require.NotContains(t, membersResponse.Body.String(), "outsider@example.test")
	for _, path := range []string{"/" + id, "/" + id + "/members"} {
		require.Equal(t, 200, request(t, router, owner, "GET", path, "").Code)
		require.Equal(t, 404, request(t, router, outsider, "GET", path, "").Code)
	}
	require.NotContains(t, request(t, router, outsider, "GET", "", "").Body.String(), id)
	require.Contains(t, request(t, router, owner, "GET", "", "").Body.String(), id)
	listed, err := NewRepository(db).List(context.Background(), owner)
	require.NoError(t, err)
	require.Len(t, listed, 1)
	require.Equal(t, *m, listed[0].Membership)
	for _, route := range []struct{ method, path, body string }{
		{"POST", "", `{"name":"Denied"}`}, {"GET", "", ""}, {"GET", "/" + id, ""},
		{"GET", "/" + id + "/members", ""}, {"POST", "/" + id + "/members", `{}`},
		{"PATCH", "/" + id + "/members/" + owner, `{}`}, {"DELETE", "/" + id + "/members/" + owner, ""},
	} {
		require.Equal(t, 401, request(t, router, "", route.method, route.path, route.body).Code)
		req := httptest.NewRequest(route.method, "/api/v1/organizations"+route.path, strings.NewReader(route.body))
		req.Header.Set("Authorization", "Bearer invalid")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		require.Equal(t, 401, w.Code)
	}
	require.Equal(t, 400, request(t, router, owner, "POST", "", `{"name":" "}`).Code)
}
