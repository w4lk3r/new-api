package controller

import (
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel/task/lingjing"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
)

var lingjingForwardHeaders = []string{
	"Content-Type",
	"Content-Length",
	"Content-Disposition",
	"Location",
	"Accept-Ranges",
	"Cache-Control",
	"ETag",
	"Last-Modified",
}

func LingjingVideoUpload(c *gin.Context) {
	if !strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "Content-Type must be multipart/form-data"})
		return
	}
	if common.GetContextKeyInt(c, constant.ContextKeyChannelType) != constant.ChannelTypeLingjingVideo {
		c.JSON(http.StatusBadRequest, gin.H{"error": "selected channel is not LingjingVideo"})
		return
	}

	baseURL := strings.TrimRight(common.GetContextKeyString(c, constant.ContextKeyChannelBaseUrl), "/")
	if baseURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Lingjing video base URL is required"})
		return
	}
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, baseURL+"/api/open/v1/uploads", c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	req.ContentLength = c.Request.ContentLength
	req.Header.Set("Content-Type", c.GetHeader("Content-Type"))
	req.Header.Set("Accept", c.GetHeader("Accept"))
	req.Header.Set("Authorization", "Bearer "+common.GetContextKeyString(c, constant.ContextKeyChannelKey))

	settingValue, _ := common.GetContextKey(c, constant.ContextKeyChannelSetting)
	proxy := ""
	if settings, ok := settingValue.(dto.ChannelSettings); ok {
		proxy = settings.Proxy
	}

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	copyLingjingHeaders(c.Writer.Header(), resp)
	c.Status(resp.StatusCode)
	c.Writer.WriteHeaderNow()
	_, _ = io.Copy(c.Writer, resp.Body)
}

func LingjingVideoFetch(c *gin.Context) {
	task, channel, ok := getOwnedLingjingTask(c)
	if !ok {
		return
	}

	key := task.PrivateData.Key
	if key == "" {
		key = channel.Key
	}
	adaptor := &lingjing.TaskAdaptor{}
	resp, err := adaptor.FetchTask(channel.GetBaseURL(), key, map[string]any{
		"task_id": task.GetUpstreamTaskID(),
	}, channel.GetSetting().Proxy)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		copyLingjingHeaders(c.Writer.Header(), resp)
		c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
		return
	}

	publicBody, err := lingjing.RewriteTaskResponse(body, task.GetUpstreamTaskID(), task.TaskID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream returned invalid JSON"})
		return
	}
	c.Data(resp.StatusCode, "application/json; charset=utf-8", publicBody)
}

func LingjingVideoDownload(c *gin.Context) {
	task, channel, ok := getOwnedLingjingTask(c)
	if !ok {
		return
	}

	key := task.PrivateData.Key
	if key == "" {
		key = channel.Key
	}
	resp, err := lingjing.FetchTaskDownload(channel.GetBaseURL(), key, task.GetUpstreamTaskID(), channel.GetSetting().Proxy)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	copyLingjingHeaders(c.Writer.Header(), resp)
	c.Status(resp.StatusCode)
	c.Writer.WriteHeaderNow()
	_, _ = io.Copy(c.Writer, resp.Body)
}

func getOwnedLingjingTask(c *gin.Context) (*model.Task, *model.Channel, bool) {
	taskID := c.Param("task_id")
	task, exists, err := model.GetByTaskId(c.GetInt("id"), taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load task"})
		return nil, nil, false
	}
	if !exists || task == nil || task.Platform != constant.TaskPlatformLingjingVideo {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return nil, nil, false
	}
	channel, err := model.GetChannelById(task.ChannelId, true)
	if err != nil || channel.Type != constant.ChannelTypeLingjingVideo {
		c.JSON(http.StatusBadGateway, gin.H{"error": "task channel is unavailable"})
		return nil, nil, false
	}
	return task, channel, true
}

func copyLingjingHeaders(destination http.Header, response *http.Response) {
	for _, name := range lingjingForwardHeaders {
		if values := response.Header.Values(name); len(values) > 0 {
			destination.Del(name)
			for _, value := range values {
				destination.Add(name, value)
			}
		}
	}
	if response.ContentLength >= 0 {
		destination.Set("Content-Length", strconv.FormatInt(response.ContentLength, 10))
	}
}
