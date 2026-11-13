package webui

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUIIsPublicButNeverFallsThroughForAPI(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Middleware())
	router.Use(func(c *gin.Context) { c.AbortWithStatus(http.StatusUnauthorized) })
	for _, tc := range []struct {
		path string
		want int
	}{{"/", 200}, {"/rules", 200}, {"/api/v1/private", 401}, {"/debug/pprof/", 401}, {"/static/missing.js", 404}} {
		req := httptest.NewRequest("GET", tc.path, nil)
		req.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("%s: %d, want %d", tc.path, w.Code, tc.want)
		}
	}
}
