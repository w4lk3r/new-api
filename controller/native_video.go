package controller

import (
	"io"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/relay/channel"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/types"

	"github.com/gin-gonic/gin"
)

var nativeVideoForwardHeaders = []string{
	"Content-Type",
	"Content-Length",
	"Content-Disposition",
	"Location",
	"Accept-Ranges",
	"Cache-Control",
	"ETag",
	"Last-Modified",
}

// NativeVideoService contains the provider-neutral native-video HTTP behavior:
// upstream-key isolation, task ownership, response-header forwarding, and
// streaming. Provider protocol details stay behind relay.NativeVideoAdaptor.
type NativeVideoService struct{}

var nativeVideoService = NativeVideoService{}

func NativeVideoUpload(c *gin.Context) {
	nativeVideoService.Upload(c)
}

func NativeVideoFetch(c *gin.Context) {
	nativeVideoService.Fetch(c)
}

func NativeVideoDownload(c *gin.Context) {
	nativeVideoService.Download(c)
}

func (NativeVideoService) Upload(c *gin.Context) {
	adaptor, ok := selectedNativeVideoAdaptor(c)
	if !ok {
		return
	}
	if !adaptor.Capabilities().Upload {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "selected native video provider does not support uploads"})
		return
	}

	info, err := relaycommon.GenRelayInfo(c, types.RelayFormatTask, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initialize upload relay"})
		return
	}
	info.InitChannelMeta(c)
	adaptor.Init(info)
	if taskErr := adaptor.ValidateUpload(c, info); taskErr != nil {
		respondTaskError(c, taskErr)
		return
	}

	response, err := adaptor.Upload(c, info)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream upload request failed"})
		return
	}
	defer response.Body.Close()

	copyNativeVideoHeaders(c.Writer.Header(), response, true)
	c.Status(response.StatusCode)
	c.Writer.WriteHeaderNow()
	_, _ = io.Copy(c.Writer, response.Body)
}

func (NativeVideoService) Fetch(c *gin.Context) {
	task, selectedChannel, adaptor, ok := getOwnedNativeVideoTask(c)
	if !ok {
		return
	}

	key := task.PrivateData.Key
	if key == "" {
		key = selectedChannel.Key
	}
	response, err := adaptor.FetchTask(selectedChannel.GetBaseURL(), key, map[string]any{
		"task_id": task.GetUpstreamTaskID(),
	}, selectedChannel.GetSetting().Proxy)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream task request failed"})
		return
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to read upstream task response"})
		return
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		copyNativeVideoHeaders(c.Writer.Header(), response, true)
		c.Data(response.StatusCode, response.Header.Get("Content-Type"), body)
		return
	}

	publicBody, err := adaptor.RewriteTaskResponse(body, task.GetUpstreamTaskID(), task.TaskID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream returned an invalid task response"})
		return
	}
	copyNativeVideoHeaders(c.Writer.Header(), response, false)
	c.Data(response.StatusCode, "application/json; charset=utf-8", publicBody)
}

func (NativeVideoService) Download(c *gin.Context) {
	task, selectedChannel, adaptor, ok := getOwnedNativeVideoTask(c)
	if !ok {
		return
	}
	if !adaptor.Capabilities().Download {
		c.JSON(http.StatusNotImplemented, gin.H{"error": "selected native video provider does not support downloads"})
		return
	}

	key := task.PrivateData.Key
	if key == "" {
		key = selectedChannel.Key
	}
	response, err := adaptor.FetchTaskDownload(selectedChannel.GetBaseURL(), key, task.GetUpstreamTaskID(), selectedChannel.GetSetting().Proxy)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream download request failed"})
		return
	}
	defer response.Body.Close()

	copyNativeVideoHeaders(c.Writer.Header(), response, true)
	c.Status(response.StatusCode)
	c.Writer.WriteHeaderNow()
	_, _ = io.Copy(c.Writer, response.Body)
}

func selectedNativeVideoAdaptor(c *gin.Context) (channel.NativeVideoAdaptor, bool) {
	if common.GetContextKeyInt(c, constant.ContextKeyChannelType) != constant.ChannelTypeNativeVideo {
		c.JSON(http.StatusBadRequest, gin.H{"error": "selected channel is not a native video channel"})
		return nil, false
	}

	adapterKey := common.GetContextKeyString(c, constant.ContextKeyChannelNativeVideoAdapter)
	if adapterKey == "" {
		// Direct handler tests and pre-migration requests may not yet carry the
		// selected-channel context key. Provider-specific paths still make the
		// required protocol unambiguous.
		adapterKey = constant.NativeVideoAdapterKeyForPath(c.Request.URL.Path)
	}
	adaptor := relay.GetNativeVideoAdaptor(adapterKey)
	if adaptor == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "selected native video adapter is unavailable"})
		return nil, false
	}
	if requiredAdapter := constant.NativeVideoAdapterKeyForPath(c.Request.URL.Path); requiredAdapter != "" && adaptor.AdapterKey() != requiredAdapter {
		c.JSON(http.StatusNotFound, gin.H{"error": "native video route is not available for the selected adapter"})
		return nil, false
	}
	return adaptor, true
}

func getOwnedNativeVideoTask(c *gin.Context) (*model.Task, *model.Channel, channel.NativeVideoAdaptor, bool) {
	taskID := c.Param("task_id")
	task, exists, err := model.GetByTaskId(c.GetInt("id"), taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load task"})
		return nil, nil, nil, false
	}
	if !exists || task == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return nil, nil, nil, false
	}

	selectedChannel, err := model.GetChannelById(task.ChannelId, true)
	if err != nil || selectedChannel.Type != constant.ChannelTypeNativeVideo {
		c.JSON(http.StatusBadGateway, gin.H{"error": "task channel is unavailable"})
		return nil, nil, nil, false
	}
	adaptor := relay.GetNativeVideoAdaptorForTask(task.Platform, selectedChannel)
	if adaptor == nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "task channel adapter is unavailable"})
		return nil, nil, nil, false
	}
	if requiredAdapter := constant.NativeVideoAdapterKeyForPath(c.Request.URL.Path); requiredAdapter != "" && adaptor.AdapterKey() != requiredAdapter {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return nil, nil, nil, false
	}
	return task, selectedChannel, adaptor, true
}

func copyNativeVideoHeaders(destination http.Header, response *http.Response, preserveContentLength bool) {
	for _, name := range nativeVideoForwardHeaders {
		if name == "Content-Length" && !preserveContentLength {
			continue
		}
		if values := response.Header.Values(name); len(values) > 0 {
			destination.Del(name)
			for _, value := range values {
				destination.Add(name, value)
			}
		}
	}
	if preserveContentLength && response.ContentLength >= 0 {
		destination.Set("Content-Length", strconv.FormatInt(response.ContentLength, 10))
	}
	if !preserveContentLength {
		destination.Del("Content-Length")
	}
}
