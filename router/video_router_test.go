package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestLingjingVideoRoutesRequireTokenAuthentication(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	SetVideoRouter(engine)

	tests := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/v1/lingjing/uploads"},
		{method: http.MethodPost, path: "/v1/lingjing/videos"},
		{method: http.MethodGet, path: "/v1/lingjing/videos/task_public"},
		{method: http.MethodGet, path: "/v1/lingjing/videos/task_public/download"},
		{method: http.MethodPost, path: "/v1/native-video/generations"},
		{method: http.MethodGet, path: "/v1/native-video/tasks/task_public"},
		{method: http.MethodGet, path: "/v1/native-video/tasks/task_public/download"},
	}

	for _, test := range tests {
		t.Run(test.method+" "+test.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(test.method, test.path, nil)
			engine.ServeHTTP(recorder, request)
			assert.Equal(t, http.StatusUnauthorized, recorder.Code)
		})
	}
}
