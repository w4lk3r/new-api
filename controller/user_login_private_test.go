package controller

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPasswordLoginDisabledPreservesAdminSecurity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousTranslate := common.TranslateMessage
	require.NoError(t, i18n.Init())
	t.Cleanup(func() { common.TranslateMessage = previousTranslate })
	for _, test := range []struct {
		name     string
		role     int
		status   int
		password string
		twoFA    bool
		success  bool
	}{
		{"normal_cannot_use_admin_query", common.RoleCommonUser, common.UserStatusEnabled, "test-password", false, false},
		{"admin", common.RoleAdminUser, common.UserStatusEnabled, "test-password", false, true},
		{"root", common.RoleRootUser, common.UserStatusEnabled, "test-password", false, true},
		{"admin_wrong_password", common.RoleAdminUser, common.UserStatusEnabled, "wrong-password", false, false},
		{"disabled_admin", common.RoleAdminUser, common.UserStatusDisabled, "test-password", false, false},
		{"admin_requires_twofa", common.RoleAdminUser, common.UserStatusEnabled, "test-password", true, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			db := setupManageUserTestDB(t)
			require.NoError(t, db.AutoMigrate(&model.TwoFA{}, &model.AuthFlow{}))
			previousEnabled, previousEncryption := common.PasswordLoginEnabled, common.PasswordLoginEncryptionEnabled
			common.PasswordLoginEnabled, common.PasswordLoginEncryptionEnabled = false, false
			t.Cleanup(func() {
				common.PasswordLoginEnabled, common.PasswordLoginEncryptionEnabled = previousEnabled, previousEncryption
			})
			hash, err := common.Password2Hash("test-password")
			require.NoError(t, err)
			user := model.User{Username: "login-private", Password: hash, Role: test.role, Status: test.status, AuthVersion: 1}
			require.NoError(t, db.Create(&user).Error)
			if test.twoFA {
				require.NoError(t, db.Create(&model.TwoFA{UserId: user.Id, Secret: "unused", IsEnabled: true}).Error)
			}
			body, err := common.Marshal(LoginRequest{Username: user.Username, Password: test.password})
			require.NoError(t, err)
			router := gin.New()
			router.POST("/api/user/login", Login)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/api/user/login?admin=true", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			router.ServeHTTP(recorder, request)
			require.Equal(t, http.StatusOK, recorder.Code)
			var response struct {
				Success bool `json:"success"`
				Data    struct {
					AccessToken string `json:"access_token"`
					Require2FA  bool   `json:"require_2fa"`
					FlowToken   string `json:"flow_token"`
				} `json:"data"`
			}
			require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response))
			assert.Equal(t, test.success, response.Success, recorder.Body.String())
			var sessions int64
			require.NoError(t, db.Model(&model.UserSession{}).Count(&sessions).Error)
			if test.success && !test.twoFA {
				assert.NotEmpty(t, response.Data.AccessToken)
				assert.EqualValues(t, 1, sessions)
			} else {
				assert.Empty(t, response.Data.AccessToken)
				assert.Zero(t, sessions)
			}
			if test.twoFA {
				assert.True(t, response.Data.Require2FA)
				assert.NotEmpty(t, response.Data.FlowToken)
				var flow model.AuthFlow
				require.NoError(t, db.First(&flow).Error)
				assert.Equal(t, user.Id, flow.UserId)
				assert.Equal(t, model.AuthFlowPurposeTwoFALogin, flow.Purpose)
			}
		})
	}
}
