package controller

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupLingjingControllerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db
	require.NoError(t, db.AutoMigrate(&model.Channel{}, &model.Task{}))
	service.InitHttpClient()
	t.Cleanup(func() {
		sqlDB, dbErr := db.DB()
		if dbErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestLingjingVideoUploadForwardsMultipartAndChannelKey(t *testing.T) {
	service.InitHttpClient()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/open/v1/uploads", r.URL.Path)
		assert.Equal(t, "Bearer upstream-secret", r.Header.Get("Authorization"))
		require.NoError(t, r.ParseMultipartForm(1<<20))
		file, header, err := r.FormFile("file")
		require.NoError(t, err)
		defer file.Close()
		assert.Equal(t, "reference.png", header.Filename)
		content := new(bytes.Buffer)
		_, err = content.ReadFrom(file)
		require.NoError(t, err)
		assert.Equal(t, "image-bytes", content.String())
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"upload_id":"upload-1"}`))
	}))
	defer upstream.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "reference.png")
	require.NoError(t, err)
	_, err = part.Write([]byte("image-bytes"))
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/lingjing/uploads", &body)
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())
	common.SetContextKey(c, constant.ContextKeyChannelType, constant.ChannelTypeLingjingVideo)
	common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, upstream.URL)
	common.SetContextKey(c, constant.ContextKeyChannelKey, "upstream-secret")
	common.SetContextKey(c, constant.ContextKeyChannelSetting, dto.ChannelSettings{})

	LingjingVideoUpload(c)
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"upload_id":"upload-1"}`, recorder.Body.String())
}

func TestValidateLingjingChannelRequiresOriginBaseURL(t *testing.T) {
	validBaseURL := "https://example.com"
	valid := &model.Channel{Type: constant.ChannelTypeNativeVideo, AdapterKey: constant.NativeVideoAdapterLingjing, Key: "key", BaseURL: &validBaseURL, Models: "lingjing-video-v1"}
	require.NoError(t, validateChannel(valid, true))

	for _, invalidBaseURL := range []string{"", "example.com", "ftp://example.com", "https://user:pass@example.com", "https://example.com/api/open/v1", "https://example.com?key=value"} {
		channel := &model.Channel{Type: constant.ChannelTypeNativeVideo, AdapterKey: constant.NativeVideoAdapterLingjing, Key: "key", BaseURL: &invalidBaseURL, Models: "lingjing-video-v1"}
		require.Error(t, validateChannel(channel, true))
	}
}

func TestLingjingTaskOwnershipBlocksOtherUsers(t *testing.T) {
	db := setupLingjingControllerTestDB(t)
	var upstreamCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		upstreamCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	channel := seedLingjingChannelAndTask(t, db, upstream.URL)
	_ = channel

	for _, handler := range []gin.HandlerFunc{LingjingVideoFetch, LingjingVideoDownload} {
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodGet, "/v1/lingjing/videos/task_public", nil)
		c.Params = gin.Params{{Key: "task_id", Value: "task_public"}}
		c.Set("id", 2)
		handler(c)
		assert.Equal(t, http.StatusNotFound, recorder.Code)
	}
	assert.Zero(t, upstreamCalls.Load())
}

func TestLingjingVideoFetchReturnsNativeStatusWithPublicTaskID(t *testing.T) {
	db := setupLingjingControllerTestDB(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/open/v1/videos/upstream-task", r.URL.Path)
		assert.Equal(t, "Bearer task-specific-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"task_id":"upstream-task","status":"processing","progress":42}}`))
	}))
	defer upstream.Close()
	seedLingjingChannelAndTask(t, db, upstream.URL)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/lingjing/videos/task_public", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task_public"}}
	c.Set("id", 1)
	LingjingVideoFetch(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"task_id":"task_public","data":{"task_id":"task_public","status":"processing","progress":42}}`, recorder.Body.String())
}

func TestNativeVideoGenericFetchUsesTaskPlatform(t *testing.T) {
	db := setupLingjingControllerTestDB(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/open/v1/videos/upstream-task", r.URL.Path)
		assert.Equal(t, "Bearer task-specific-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"task_id":"upstream-task","status":"processing"}`))
	}))
	defer upstream.Close()
	seedLingjingChannelAndTask(t, db, upstream.URL)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/native-video/tasks/task_public", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task_public"}}
	c.Set("id", 1)
	NativeVideoFetch(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.JSONEq(t, `{"task_id":"task_public","status":"processing"}`, recorder.Body.String())
}

func TestLingjingVideoDownloadStreamsMP4AndPreservesRedirect(t *testing.T) {
	t.Run("mp4", func(t *testing.T) {
		db := setupLingjingControllerTestDB(t)
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Content-Disposition", `attachment; filename="video.mp4"`)
			w.Header().Set("Content-Length", "11")
			_, _ = w.Write([]byte("video-bytes"))
		}))
		defer upstream.Close()
		seedLingjingChannelAndTask(t, db, upstream.URL)

		recorder := executeLingjingDownload(t)
		assert.Equal(t, http.StatusOK, recorder.Code)
		assert.Equal(t, "video/mp4", recorder.Header().Get("Content-Type"))
		assert.Equal(t, "11", recorder.Header().Get("Content-Length"))
		assert.Equal(t, `attachment; filename="video.mp4"`, recorder.Header().Get("Content-Disposition"))
		assert.Equal(t, "video-bytes", recorder.Body.String())
	})

	t.Run("redirect", func(t *testing.T) {
		db := setupLingjingControllerTestDB(t)
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Location", "https://cdn.example.com/video.mp4")
			w.WriteHeader(http.StatusFound)
		}))
		defer upstream.Close()
		seedLingjingChannelAndTask(t, db, upstream.URL)

		recorder := executeLingjingDownload(t)
		assert.Equal(t, http.StatusFound, recorder.Code)
		assert.Equal(t, "https://cdn.example.com/video.mp4", recorder.Header().Get("Location"))
	})
}

func seedLingjingChannelAndTask(t *testing.T, db *gorm.DB, baseURL string) *model.Channel {
	t.Helper()
	channel := &model.Channel{
		Type:       constant.ChannelTypeNativeVideo,
		AdapterKey: constant.NativeVideoAdapterLingjing,
		Key:        "channel-key",
		Status:     common.ChannelStatusEnabled,
		Name:       "lingjing-test",
		BaseURL:    &baseURL,
		Models:     "lingjing-video-v1",
		Group:      "default",
	}
	require.NoError(t, db.Create(channel).Error)
	task := &model.Task{
		TaskID:    "task_public",
		Platform:  constant.TaskPlatformLingjingVideo,
		UserId:    1,
		ChannelId: channel.Id,
		Status:    model.TaskStatusInProgress,
		PrivateData: model.TaskPrivateData{
			UpstreamTaskID: "upstream-task",
			Key:            "task-specific-key",
		},
	}
	require.NoError(t, db.Create(task).Error)
	return channel
}

func executeLingjingDownload(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/v1/lingjing/videos/task_public/download", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task_public"}}
	c.Set("id", 1)
	LingjingVideoDownload(c)
	return recorder
}
