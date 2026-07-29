package lingjing

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/tidwall/sjson"
)

const (
	ChannelName       = "lingjing-video"
	DefaultModel      = "lingjing-video-v1"
	SupportedDuration = 15
	upstreamVideoPath = "/api/open/v1/videos"
)

var ModelList = []string{DefaultModel}

type createRequest struct {
	Model    string `json:"model"`
	Duration *int   `json:"duration"`
}

type TaskAdaptor struct {
	taskcommon.BaseBilling
	apiKey  string
	baseURL string
}

func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.apiKey = info.ApiKey
	a.baseURL = strings.TrimRight(info.ChannelBaseUrl, "/")
}

func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if c.Request.Method != http.MethodPost || c.Request.URL.Path != "/v1/lingjing/videos" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("unsupported Lingjing video operation"), "invalid_request", http.StatusBadRequest)
	}
	if !strings.HasPrefix(c.GetHeader("Content-Type"), "application/json") {
		return service.TaskErrorWrapperLocal(fmt.Errorf("Content-Type must be application/json"), "invalid_request", http.StatusUnsupportedMediaType)
	}

	var request createRequest
	if err := common.UnmarshalBodyReusable(c, &request); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if request.Duration == nil {
		return service.TaskErrorWrapperLocal(fmt.Errorf("duration is required"), "invalid_duration", http.StatusBadRequest)
	}
	duration := *request.Duration
	if duration <= 0 || duration > relaycommon.MaxTaskDurationSeconds {
		return service.TaskErrorWrapperLocal(fmt.Errorf("duration must be between 1 and %d", relaycommon.MaxTaskDurationSeconds), "invalid_duration", http.StatusBadRequest)
	}
	if duration != SupportedDuration {
		return service.TaskErrorWrapperLocal(fmt.Errorf("duration must be %d", SupportedDuration), "invalid_duration", http.StatusBadRequest)
	}
	if strings.TrimSpace(request.Model) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("model is required"), "invalid_request", http.StatusBadRequest)
	}

	c.Set("lingjing_video_request", request)
	if info.TaskRelayInfo == nil {
		info.TaskRelayInfo = &relaycommon.TaskRelayInfo{}
	}
	info.Action = constant.TaskActionGenerate
	return nil
}

func (a *TaskAdaptor) EstimateBilling(c *gin.Context, _ *relaycommon.RelayInfo) map[string]float64 {
	requestValue, exists := c.Get("lingjing_video_request")
	if !exists {
		return nil
	}
	request, ok := requestValue.(createRequest)
	if !ok || request.Duration == nil {
		return nil
	}
	return map[string]float64{"seconds": float64(*request.Duration)}
}

func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	if a.baseURL == "" {
		return "", fmt.Errorf("Lingjing video base URL is required")
	}
	return a.baseURL + upstreamVideoPath, nil
}

func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return nil
}

func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, errors.Wrap(err, "get request body")
	}
	body, err := storage.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "read request body")
	}
	body, err = sjson.SetBytes(body, "model", info.UpstreamModelName)
	if err != nil {
		return nil, errors.Wrap(err, "map upstream model")
	}
	return bytes.NewReader(body), nil
}

func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (string, []byte, *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var payload any
	if err := common.Unmarshal(responseBody, &payload); err != nil {
		return "", nil, service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusBadGateway)
	}
	upstreamTaskID := findStringField(payload, "task_id", "id")
	if upstreamTaskID == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusBadGateway)
	}

	publicPayload := replaceTaskID(payload, upstreamTaskID, info.PublicTaskID)
	if object, ok := publicPayload.(map[string]any); ok {
		object["task_id"] = info.PublicTaskID
	}
	publicBody, err := common.Marshal(publicPayload)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	c.Data(resp.StatusCode, "application/json; charset=utf-8", publicBody)
	return upstreamTaskID, responseBody, nil
}

func (a *TaskAdaptor) FetchTask(baseURL, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok || strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	requestURL := strings.TrimRight(baseURL, "/") + upstreamVideoPath + "/" + url.PathEscape(taskID)
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

func FetchTaskDownload(baseURL, key, taskID, proxy string) (*http.Response, error) {
	if strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("invalid task_id")
	}
	requestURL := strings.TrimRight(baseURL, "/") + upstreamVideoPath + "/" + url.PathEscape(taskID) + "/download"
	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	noRedirectClient := *client
	noRedirectClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return noRedirectClient.Do(req)
}

func RewriteTaskResponse(responseBody []byte, upstreamTaskID, publicTaskID string) ([]byte, error) {
	var payload any
	if err := common.Unmarshal(responseBody, &payload); err != nil {
		return nil, err
	}
	payload = replaceTaskID(payload, upstreamTaskID, publicTaskID)
	if object, ok := payload.(map[string]any); ok {
		object["task_id"] = publicTaskID
	}
	return common.Marshal(payload)
}

func (a *TaskAdaptor) ParseTaskResult(responseBody []byte) (*relaycommon.TaskInfo, error) {
	var payload any
	if err := common.Unmarshal(responseBody, &payload); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result")
	}

	result := &relaycommon.TaskInfo{
		TaskID: findStringField(payload, "task_id", "id"),
		Url:    findStringField(payload, "download_url", "video_url", "url"),
	}
	status := strings.ToLower(findStringField(payload, "status", "state"))
	switch status {
	case "submitted", "created":
		result.Status = model.TaskStatusSubmitted
		result.Progress = taskcommon.ProgressSubmitted
	case "pending", "queued", "queueing":
		result.Status = model.TaskStatusQueued
		result.Progress = taskcommon.ProgressQueued
	case "processing", "running", "in_progress", "in-progress":
		result.Status = model.TaskStatusInProgress
		result.Progress = taskcommon.ProgressInProgress
	case "success", "succeeded", "completed", "complete", "done":
		result.Status = model.TaskStatusSuccess
		result.Progress = taskcommon.ProgressComplete
	case "failure", "failed", "error", "cancelled", "canceled", "timeout", "timed_out":
		result.Status = model.TaskStatusFailure
		result.Progress = taskcommon.ProgressComplete
		result.Reason = findStringField(payload, "fail_reason", "error_message", "message", "reason")
	default:
		result.Status = model.TaskStatusInProgress
		result.Progress = taskcommon.ProgressInProgress
	}
	return result, nil
}

func (a *TaskAdaptor) GetModelList() []string { return ModelList }

func (a *TaskAdaptor) GetChannelName() string { return ChannelName }

func findStringField(value any, keys ...string) string {
	object, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	for _, key := range keys {
		if field, ok := object[key].(string); ok && field != "" {
			return field
		}
	}
	for _, nestedKey := range []string{"data", "result", "task", "error"} {
		if nested, exists := object[nestedKey]; exists {
			if field := findStringField(nested, keys...); field != "" {
				return field
			}
		}
	}
	return ""
}

func replaceTaskID(value any, upstreamTaskID, publicTaskID string) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if stringValue, ok := child.(string); ok && stringValue == upstreamTaskID && (key == "id" || key == "task_id") {
				typed[key] = publicTaskID
				continue
			}
			typed[key] = replaceTaskID(child, upstreamTaskID, publicTaskID)
		}
	case []any:
		for index, child := range typed {
			typed[index] = replaceTaskID(child, upstreamTaskID, publicTaskID)
		}
	}
	return value
}
