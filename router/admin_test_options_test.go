package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAdminTestOptionsRequiresAdminAndReturnsOnlyRoutingMetadata(t *testing.T) {
	setupRelayRouterTestDB(t)
	require.NoError(t, model.DB.AutoMigrate(&model.Channel{}))
	for _, user := range []model.User{
		{Id: 91001, Username: "catalog-root", AffCode: "catalogroot", Role: common.RoleRootUser, Status: common.UserStatusEnabled, Group: "default", Quota: 100},
		{Id: 91002, Username: "catalog-user", AffCode: "cataloguser", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", Quota: 100},
	} {
		require.NoError(t, model.DB.Create(&user).Error)
	}
	for _, token := range []model.Token{
		{UserId: 91001, Key: "catalogroot", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true},
		{UserId: 91002, Key: "cataloguser", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true},
		{UserId: 91001, Key: "cataloglimited", Status: common.TokenStatusEnabled, ExpiredTime: -1, UnlimitedQuota: true, ModelLimitsEnabled: true, ModelLimits: "deepseek-test"},
		{UserId: 91001, Key: "catalogdisabled", Status: common.TokenStatusDisabled, ExpiredTime: -1, UnlimitedQuota: true},
	} {
		require.NoError(t, model.DB.Create(&token).Error)
	}
	for _, channel := range []model.Channel{
		{Id: 91001, Name: "test provider", Type: 1, Status: common.ChannelStatusEnabled, Group: "admin", Models: "deepseek-test,deepseek-other", Key: "upstream-secret"},
		{Id: 91002, Name: "disabled provider", Type: 1, Status: common.ChannelStatusManuallyDisabled, Group: "default", Models: "disabled-model", Key: "disabled-secret"},
	} {
		require.NoError(t, model.DB.Create(&channel).Error)
	}
	engine := gin.New()
	SetRelayRouter(engine)
	for _, test := range []struct {
		name   string
		key    string
		status int
		admin  bool
		models []string
	}{
		{name: "missing token", status: http.StatusUnauthorized},
		{name: "disabled token", key: "catalogdisabled", status: http.StatusUnauthorized},
		{name: "ordinary user", key: "cataloguser", status: http.StatusOK},
		{name: "root", key: "catalogroot", status: http.StatusOK, admin: true, models: []string{"deepseek-test", "deepseek-other"}},
		{name: "limited root token", key: "cataloglimited", status: http.StatusOK, admin: true, models: []string{"deepseek-test"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/v1/admin/test-options", nil)
			if test.key != "" {
				request.Header.Set("Authorization", "Bearer sk-"+test.key)
			}
			engine.ServeHTTP(recorder, request)
			require.Equal(t, test.status, recorder.Code, recorder.Body.String())
			assert.NotContains(t, recorder.Body.String(), "upstream-secret")
			assert.NotContains(t, recorder.Body.String(), "disabled-secret")
			if test.status != http.StatusOK {
				return
			}
			var response struct {
				Success bool `json:"success"`
				Data    struct {
					IsAdmin  bool `json:"is_admin"`
					Channels []struct {
						ID     int      `json:"id"`
						Models []string `json:"models"`
					} `json:"channels"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.True(t, response.Success)
			assert.Equal(t, test.admin, response.Data.IsAdmin)
			assert.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
			if !test.admin {
				assert.Empty(t, response.Data.Channels)
				return
			}
			require.Len(t, response.Data.Channels, 1)
			assert.Equal(t, 91001, response.Data.Channels[0].ID)
			assert.Equal(t, test.models, response.Data.Channels[0].Models)
		})
	}
}
