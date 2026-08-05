package lingjing

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func newCreateContext(body string) *gin.Context {
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/lingjing/videos", bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c
}

func TestValidateRequestAndEstimateBilling(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c := newCreateContext(`{"model":"lingjing-video-15s","duration":15,"prompt":"sunrise"}`)
	info := &relaycommon.RelayInfo{}
	adaptor := &TaskAdaptor{}

	require.Nil(t, adaptor.ValidateRequestAndSetAction(c, info))
	assert.Equal(t, map[string]float64{"seconds": 15}, adaptor.EstimateBilling(c, info))
}

func TestValidateGenericNativeVideoCreateRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{
		`{"model":"video-standard-15s","duration":15,"prompt":"sunrise"}`,
		`{"duration":15,"prompt":"sunrise"}`,
	} {
		c := newCreateContext(body)
		c.Request.URL.Path = "/v1/native-video/generations"
		adaptor := &TaskAdaptor{}

		require.Nil(t, adaptor.ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{}))
		assert.True(t, adaptor.SupportsCreateRequest(c))
		assert.Equal(t, constant.NativeVideoAdapterLingjing, adaptor.AdapterKey())
	}
}

func TestValidateRequestRejectsUnsupportedDuration(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, body := range []string{
		`{"model":"lingjing-video-v1"}`,
		`{"model":"lingjing-video-v1","duration":0}`,
		`{"model":"lingjing-video-v1","duration":14}`,
		`{"model":"lingjing-video-v1","duration":16}`,
		`{"model":"lingjing-video-v1","duration":999999999}`,
	} {
		c := newCreateContext(body)
		taskErr := (&TaskAdaptor{}).ValidateRequestAndSetAction(c, &relaycommon.RelayInfo{})
		require.NotNil(t, taskErr)
		assert.Equal(t, http.StatusBadRequest, taskErr.StatusCode)
	}
}

func TestBuildRequestBodyPreservesNativeFieldsAndMapsModel(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := `{"model":"lingjing-video-15s","model_key":"relay","prompt":"sunrise","duration":15,"ratio":"16:9","reference_images":["image-1"],"reference_videos":["video-1"],"reference_audios":["audio-1"],"generate_audio":false,"future_field":{"enabled":true}}`
	c := newCreateContext(body)
	adaptor := &TaskAdaptor{}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{UpstreamModelName: DefaultModel}}
	reader, err := adaptor.BuildRequestBody(c, info)
	require.NoError(t, err)
	forwarded, err := io.ReadAll(reader)
	require.NoError(t, err)

	assert.Equal(t, DefaultModel, gjson.GetBytes(forwarded, "model").String())
	assert.Equal(t, "relay", gjson.GetBytes(forwarded, "model_key").String())
	assert.Equal(t, "sunrise", gjson.GetBytes(forwarded, "prompt").String())
	assert.EqualValues(t, 15, gjson.GetBytes(forwarded, "duration").Int())
	assert.Equal(t, "16:9", gjson.GetBytes(forwarded, "ratio").String())
	assert.Equal(t, "image-1", gjson.GetBytes(forwarded, "reference_images.0").String())
	assert.Equal(t, "video-1", gjson.GetBytes(forwarded, "reference_videos.0").String())
	assert.Equal(t, "audio-1", gjson.GetBytes(forwarded, "reference_audios.0").String())
	assert.False(t, gjson.GetBytes(forwarded, "generate_audio").Bool())
	assert.True(t, gjson.GetBytes(forwarded, "future_field.enabled").Bool())
}

func TestDoRequestUsesNativePathAndChannelAuthorization(t *testing.T) {
	service.InitHttpClient()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/open/v1/videos", r.URL.Path)
		assert.Equal(t, "Bearer upstream-secret", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"task_id":"upstream-task"}`))
	}))
	defer upstream.Close()

	c := newCreateContext(`{"model":"lingjing-video-v1","duration":15}`)
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelBaseUrl: upstream.URL,
			ApiKey:         "upstream-secret",
		},
	}
	adaptor := &TaskAdaptor{}
	adaptor.Init(info)
	resp, err := adaptor.DoRequest(c, info, bytes.NewBufferString(`{"duration":15}`))
	require.NoError(t, err)
	require.NotNil(t, resp)
	_ = resp.Body.Close()
}

func TestDoResponseReplacesUpstreamTaskID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	resp := &http.Response{
		StatusCode: http.StatusCreated,
		Body:       io.NopCloser(bytes.NewBufferString(`{"data":{"task_id":"upstream-task","status":"queued"}}`)),
	}

	upstreamID, stored, taskErr := (&TaskAdaptor{}).DoResponse(c, resp, &relaycommon.RelayInfo{TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"}})
	require.Nil(t, taskErr)
	assert.Equal(t, "upstream-task", upstreamID)
	assert.Contains(t, string(stored), "upstream-task")
	assert.Equal(t, http.StatusCreated, recorder.Code)
	assert.Equal(t, "task_public", gjson.Get(recorder.Body.String(), "task_id").String())
	assert.Equal(t, "task_public", gjson.Get(recorder.Body.String(), "data.task_id").String())
	assert.NotContains(t, recorder.Body.String(), "upstream-task")
}

func TestInitTaskKeepsPublicIDAndSelectedChannelKeyPrivate(t *testing.T) {
	info := &relaycommon.RelayInfo{
		UserId: 7,
		ChannelMeta: &relaycommon.ChannelMeta{
			ChannelType:       constant.ChannelTypeLingjingVideo,
			ChannelId:         11,
			ApiKey:            "selected-upstream-key",
			UpstreamModelName: DefaultModel,
		},
		TaskRelayInfo: &relaycommon.TaskRelayInfo{PublicTaskID: "task_public"},
	}
	info.OriginModelName = "lingjing-video-15s"
	task := model.InitTask(constant.TaskPlatformLingjingVideo, info)

	assert.Equal(t, "task_public", task.TaskID)
	assert.Equal(t, 7, task.UserId)
	assert.Equal(t, 11, task.ChannelId)
	assert.Equal(t, "selected-upstream-key", task.PrivateData.Key)
	assert.Equal(t, "lingjing-video-15s", task.Properties.OriginModelName)
	assert.Equal(t, DefaultModel, task.Properties.UpstreamModelName)
}

func TestParseTaskResultMapsStatuses(t *testing.T) {
	tests := []struct {
		status string
		want   model.TaskStatus
	}{
		{status: "queued", want: model.TaskStatusQueued},
		{status: "processing", want: model.TaskStatusInProgress},
		{status: "succeeded", want: model.TaskStatusSuccess},
		{status: "failed", want: model.TaskStatusFailure},
		{status: "cancelled", want: model.TaskStatusFailure},
		{status: "timeout", want: model.TaskStatusFailure},
	}
	for _, test := range tests {
		t.Run(test.status, func(t *testing.T) {
			result, err := (&TaskAdaptor{}).ParseTaskResult([]byte(`{"data":{"task_id":"upstream-task","status":"` + test.status + `","video_url":"https://example.com/video.mp4","message":"failed upstream"}}`))
			require.NoError(t, err)
			assert.Equal(t, string(test.want), result.Status)
			if test.want == model.TaskStatusFailure {
				assert.Equal(t, "failed upstream", result.Reason)
			}
		})
	}
}

func TestFetchTaskDownloadDoesNotFollowRedirect(t *testing.T) {
	service.InitHttpClient()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/open/v1/videos/upstream-task/download", r.URL.Path)
		assert.Equal(t, "Bearer upstream-secret", r.Header.Get("Authorization"))
		w.Header().Set("Location", "https://cdn.example.com/video.mp4")
		w.WriteHeader(http.StatusFound)
	}))
	defer upstream.Close()

	resp, err := FetchTaskDownload(upstream.URL, "upstream-secret", "upstream-task", "")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusFound, resp.StatusCode)
	assert.Equal(t, "https://cdn.example.com/video.mp4", resp.Header.Get("Location"))
}

func TestRewriteTaskResponseRejectsInvalidJSON(t *testing.T) {
	_, err := RewriteTaskResponse([]byte("not-json"), "upstream-task", "task_public")
	require.Error(t, err)
}

func TestNativeResponseJSONUsesProjectWrapper(t *testing.T) {
	response, err := RewriteTaskResponse([]byte(`{"task_id":"upstream-task"}`), "upstream-task", "task_public")
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, common.Unmarshal(response, &decoded))
	assert.Equal(t, "task_public", decoded["task_id"])
}
