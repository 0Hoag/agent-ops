package response

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestSuccessAndErrorFixtures(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/ok", func(c *gin.Context) { OK(c, gin.H{"id": 7}) })
	router.GET("/error", func(c *gin.Context) { NotFound(c, "missing") })
	for _, test := range []struct {
		path, want string
		status     int
	}{
		{"/ok", `{"status":200,"data":{"id":7}}`, 200},
		{"/error", `{"status":404,"message":"missing"}`, 404},
	} {
		t.Run(test.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest("GET", test.path, nil))
			require.Equal(t, test.status, recorder.Code)
			require.JSONEq(t, test.want, recorder.Body.String())
		})
	}
}
