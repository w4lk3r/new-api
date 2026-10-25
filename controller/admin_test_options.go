package controller

import (
	"net/http"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service/authz"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/gin-gonic/gin"
)

// GetAdminTestOptions exposes only routing labels, never upstream credentials.
// TokenAuth validates the same token/IP/group/quota rules as relay requests.
func GetAdminTestOptions(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	type testChannel struct {
		ID     int      `json:"id"`
		Name   string   `json:"name"`
		Groups []string `json:"groups"`
		Models []string `json:"models"`
	}
	channels := make([]testChannel, 0)
	user, err := model.GetUserCache(c.GetInt("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Unable to load test options"})
		return
	}
	allowed := user.Role >= common.RoleAdminUser &&
		authz.Can(user.Id, user.Role, authz.ChannelRead) &&
		authz.Can(user.Id, user.Role, authz.ChannelOperate)
	if !allowed {
		c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"is_admin": false, "channels": channels}})
		return
	}
	available, err := model.GetAllChannels(0, -1, false, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": "Unable to load test options"})
		return
	}
	limits, _ := common.GetContextKey(c, constant.ContextKeyTokenModelLimit)
	modelLimits, _ := limits.(map[string]bool)
	for _, channel := range available {
		if channel.Status != common.ChannelStatusEnabled {
			continue
		}
		models := make([]string, 0)
		for _, name := range channel.GetModels() {
			name = strings.TrimSpace(name)
			if name == "" || slices.Contains(models, name) {
				continue
			}
			if common.GetContextKeyBool(c, constant.ContextKeyTokenModelLimitEnabled) && !modelLimits[ratio_setting.FormatMatchingModelName(name)] {
				continue
			}
			ok, _ := model.ChannelSatisfiesFilters(channel, name, []dto.ChannelFilter{{Kind: dto.FilterRequestPath, RequestPath: "/v1/chat/completions"}})
			if ok {
				models = append(models, name)
			}
		}
		if len(models) > 0 {
			channels = append(channels, testChannel{ID: channel.Id, Name: channel.Name, Groups: channel.GetGroups(), Models: models})
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": gin.H{"is_admin": true, "channels": channels}})
}
