package controller_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	appI18n "github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/model"
	pluginruntime "github.com/QuantumNous/new-api/pkg/jsplugin"
	"github.com/QuantumNous/new-api/relay"
	"github.com/QuantumNous/new-api/router"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting"
	"github.com/QuantumNous/new-api/setting/billing_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
	"github.com/QuantumNous/new-api/setting/system_setting"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

const customVideoModel = "custom-video-30s"

func TestCustomVideoPluginOriginalFixturesAndPricingContract(t *testing.T) {
	source, err := os.ReadFile("testdata/custom-video/custom-video.plugin.js")
	require.NoError(t, err)
	fixtures, err := os.ReadFile("testdata/custom-video/custom-video.fixture.json")
	require.NoError(t, err)
	report, err := pluginruntime.ReplayFixture(context.Background(), string(source), fixtures)
	require.NoError(t, err)
	assert.Equal(t, pluginruntime.FixtureReport{Total: 9, Passed: 9}, report)
	plugin, err := pluginruntime.CompilePlugin(string(source), pluginruntime.Options{})
	require.NoError(t, err)
	// The delivered plugin does not expose seconds as a billing fact. A model
	// price is a whole-task price; the outgoing seconds field is not a meter.
	require.ErrorContains(t, billing_setting.SmokeTestTaskExpr(`u("seconds") * 0.1`, plugin.Meta.UsageSchema), "not declared")
	require.NoError(t, billing_setting.SmokeTestTaskExpr(`tier("fixed", 0.25)`, plugin.Meta.UsageSchema))
}

type customVideoUpload struct {
	idempotency string
	fields      map[string][]string
	images      map[string][]byte
}

func TestCustomVideoPluginSeededTaskLifecycle(t *testing.T) {
	runCustomVideoLifecycle(t, "seeded")
}

// Exercise the production submit path against a real HTTP multipart parser.
func TestCustomVideoPluginSubmitLifecycle(t *testing.T) {
	for _, format := range []string{"multipart", "multipart_no_images", "multipart_one_image", "multipart_three_images", "json"} {
		t.Run(format, func(t *testing.T) { runCustomVideoLifecycle(t, format) })
	}
}

func runCustomVideoLifecycle(t *testing.T, format string) {
	t.Helper()
	seeded := format == "seeded"
	gin.SetMode(gin.TestMode)
	require.NoError(t, appI18n.Init())
	service.InitHttpClient()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMainType, previousLogType := common.MainDatabaseType(), common.LogDatabaseType()
	previousSQLitePath, previousMaster := common.SQLitePath, common.IsMasterNode
	previousCache, previousRedis := common.MemoryCacheEnabled, common.RedisEnabled
	previousBatch, previousConsume := common.BatchUpdateEnabled, common.LogConsumeEnabled
	previousRateLimit := setting.ModelRequestRateLimitEnabled
	previousTaskQueryLimit := constant.TaskQueryLimit
	previousRegistry := pluginruntime.DefaultRegistry
	previousAdaptorFactory := service.GetTaskAdaptorFunc
	previousFetch := *system_setting.GetFetchSetting()
	previousPrices, previousGroups := ratio_setting.ModelPrice2JSONString(), ratio_setting.GroupRatio2JSONString()
	common.SQLitePath, common.IsMasterNode = filepath.Join(t.TempDir(), "custom-video.db"), false
	t.Setenv("SQL_DSN", "")
	t.Setenv("LOG_SQL_DSN", "")
	require.NoError(t, model.InitDB())
	database := model.DB
	sqlDB, err := database.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	model.DB, model.LOG_DB = database, database
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.MemoryCacheEnabled, common.RedisEnabled = false, false
	common.BatchUpdateEnabled, common.LogConsumeEnabled = false, true
	setting.ModelRequestRateLimitEnabled = false
	constant.TaskQueryLimit = 100
	pluginruntime.DefaultRegistry = pluginruntime.NewRegistry()
	service.GetTaskAdaptorFunc = func(platform constant.TaskPlatform) service.TaskPollingAdaptor { return relay.GetTaskAdaptor(platform) }
	// Only test-owned loopback servers are used. Production SSRF defaults stay unchanged.
	system_setting.GetFetchSetting().AllowPrivateIp = true
	system_setting.GetFetchSetting().AllowedPorts = []string{"1-65535"}
	require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(`{"custom-video-30s":0.25}`))
	require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(`{"default":2,"unserved":1}`))
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMainType, previousLogType)
		common.SQLitePath, common.IsMasterNode = previousSQLitePath, previousMaster
		common.MemoryCacheEnabled, common.RedisEnabled = previousCache, previousRedis
		common.BatchUpdateEnabled, common.LogConsumeEnabled = previousBatch, previousConsume
		setting.ModelRequestRateLimitEnabled = previousRateLimit
		constant.TaskQueryLimit = previousTaskQueryLimit
		pluginruntime.DefaultRegistry = previousRegistry
		service.GetTaskAdaptorFunc = previousAdaptorFactory
		*system_setting.GetFetchSetting() = previousFetch
		require.NoError(t, ratio_setting.UpdateModelPriceByJSONString(previousPrices))
		require.NoError(t, ratio_setting.UpdateGroupRatioByJSONString(previousGroups))
		require.NoError(t, sqlDB.Close())
	})
	require.NoError(t, database.AutoMigrate(&model.User{}, &model.Token{}, &model.Channel{}, &model.Ability{}, &model.Task{}, &model.Log{}, &model.UserSubscription{}))
	source, err := os.ReadFile("testdata/custom-video/custom-video.plugin.js")
	require.NoError(t, err)
	_, err = pluginruntime.DefaultRegistry.Register(string(source), pluginruntime.Options{})
	require.NoError(t, err)

	const initialQuota = 10_000_000
	owner := model.User{Username: "custom-video-owner", AffCode: "video-owner", Group: "default", Quota: initialQuota, Status: common.UserStatusEnabled, Role: common.RoleCommonUser, Setting: `{"billing_preference":"wallet_only"}`}
	other := model.User{Username: "custom-video-other", AffCode: "video-other", Group: "default", Quota: initialQuota, Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	unserved := model.User{Username: "custom-video-unserved", AffCode: "video-unserved", Group: "unserved", Quota: initialQuota, Status: common.UserStatusEnabled, Role: common.RoleCommonUser}
	require.NoError(t, database.Create(&owner).Error)
	require.NoError(t, database.Create(&other).Error)
	require.NoError(t, database.Create(&unserved).Error)
	ownerToken := model.Token{UserId: owner.Id, Key: "customvideoowner", Name: "owner", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: initialQuota, ModelLimitsEnabled: true, ModelLimits: customVideoModel}
	otherToken := model.Token{UserId: other.Id, Key: "customvideoother", Name: "other", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: initialQuota}
	deniedToken := model.Token{UserId: owner.Id, Key: "customvideodenied", Name: "model-denied", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: initialQuota, ModelLimitsEnabled: true, ModelLimits: "another-model"}
	unservedToken := model.Token{UserId: unserved.Id, Key: "customvideounserved", Name: "group-denied", Status: common.TokenStatusEnabled, ExpiredTime: -1, RemainQuota: initialQuota}
	for _, token := range []*model.Token{&ownerToken, &otherToken, &deniedToken, &unservedToken} {
		require.NoError(t, database.Create(token).Error)
	}

	var submitCalls, queryCalls, contentCalls atomic.Int32
	var status atomic.Value
	status.Store("processing")
	uploads := make(chan customVideoUpload, 4)
	videoBytes := []byte("test-video-content")
	media := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentCalls.Add(1)
		assert.Empty(t, r.Header.Get("Authorization"))
		assert.Empty(t, r.Header.Get("Cookie"))
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Length", fmt.Sprint(len(videoBytes)))
		_, _ = w.Write(videoBytes)
	}))
	defer media.Close()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer channel-only-key", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/videos":
			index := submitCalls.Add(1)
			if !assert.NoError(t, r.ParseMultipartForm(1<<20)) {
				http.Error(w, "multipart", 400)
				return
			}
			defer r.MultipartForm.RemoveAll()
			seen := customVideoUpload{idempotency: r.Header.Get("Idempotency-Key"), fields: r.MultipartForm.Value, images: map[string][]byte{}}
			for field, files := range r.MultipartForm.File {
				assert.Equal(t, "images[]", field)
				for _, header := range files {
					file, openErr := header.Open()
					if !assert.NoError(t, openErr) {
						continue
					}
					data, readErr := io.ReadAll(file)
					file.Close()
					assert.NoError(t, readErr)
					seen.images[header.Filename] = data
				}
			}
			uploads <- seen
			_, _ = fmt.Fprintf(w, `{"code":"1","task_id":"private-%d","status":"queued"}`, index)
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/videos/private-"):
			queryCalls.Add(1)
			taskStatus := status.Load().(string)
			code := "0"
			result := map[string]any{"task_id": strings.TrimPrefix(r.URL.Path, "/api/v1/videos/"), "status": taskStatus, "billing_state": "charged"}
			if taskStatus == "succeeded" {
				code = "1"
				result["url"] = media.URL + "/video.mp4"
			} else if taskStatus == "failed" {
				result["error"] = "provider generation failed"
				result["billing_state"] = "refunded"
			}
			data, marshalErr := common.Marshal(map[string]any{"code": code, "task": result})
			if !assert.NoError(t, marshalErr) {
				http.Error(w, "encode task", 500)
				return
			}
			_, _ = w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	channelSetting := `{"task_plugin_key":"custom-video"}`
	channel := model.Channel{Type: constant.ChannelTypeTaskPlugin, Name: "custom-video-local", Key: "channel-only-key", BaseURL: &upstream.URL, Models: customVideoModel, Group: "default", Status: common.ChannelStatusEnabled, Setting: &channelSetting}
	require.NoError(t, database.Create(&channel).Error)
	require.NoError(t, database.Create(&model.Ability{Group: "default", Model: customVideoModel, ChannelId: channel.Id, Enabled: true}).Error)
	engine := gin.New()
	router.SetTaskPluginProtocolRouter(engine)
	engine.NoRoute(router.SetPluginRouter(engine))

	for _, tc := range []struct {
		name, key, body string
		code            int
	}{
		{"missing token", "", `{"model":"custom-video-30s","prompt":"x","ratio":"16:9"}`, 401},
		{"model denied", deniedToken.Key, `{"model":"custom-video-30s","prompt":"x","ratio":"16:9"}`, 403},
		{"group unavailable", unservedToken.Key, `{"model":"custom-video-30s","prompt":"x","ratio":"16:9"}`, 503},
		{"invalid seconds", ownerToken.Key, `{"model":"custom-video-30s","prompt":"x","ratio":"16:9","seconds":"15"}`, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(tc.body))
			request.Header.Set("Content-Type", "application/json")
			response := customVideoRequest(engine, request, tc.key)
			assert.Equal(t, tc.code, response.Code, response.Body.String())
		})
	}
	assert.Zero(t, submitCalls.Load())

	var receipt struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	charge := common.QuotaFromFloat(0.25 * 2 * common.QuotaPerUnit)
	if seeded {
		// Existing paid tasks must remain queryable without resubmission.
		receipt.ID = seedCustomVideoTask(t, database, owner.Id, ownerToken, channel, "private-1", charge)
	} else {
		fields := []string{"image_1", "image_2"}
		expectedImages := map[string][]byte{"first.png": []byte("first-image"), "second.png": []byte("second-image")}
		switch format {
		case "multipart_no_images", "json":
			fields = nil
			expectedImages = map[string][]byte{}
		case "multipart_one_image":
			fields = []string{"image_1"}
			delete(expectedImages, "second.png")
		case "multipart_three_images":
			fields = append(fields, "image_3")
			expectedImages["third.png"] = []byte("third-image")
		}
		request := customVideoMultipartRequest(t, fields...)
		if format == "json" {
			request = httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"custom-video-30s","prompt":"two distinct references","ratio":"16:9","seconds":"30"}`))
			request.Header.Set("Content-Type", "application/json")
		}
		created := customVideoRequest(engine, request, ownerToken.Key)
		if created.Code != http.StatusOK {
			// Failed submissions refund asynchronously. Wait for the real funding
			// and token updates before cleanup closes the test database.
			require.Eventually(t, func() bool {
				var refundedUser model.User
				var refundedToken model.Token
				return database.First(&refundedUser, owner.Id).Error == nil &&
					database.First(&refundedToken, ownerToken.Id).Error == nil &&
					refundedUser.Quota == initialQuota && refundedToken.RemainQuota == initialQuota
			}, 3*time.Second, 10*time.Millisecond)
		}
		require.Equal(t, http.StatusOK, created.Code, created.Body.String())
		require.NoError(t, common.Unmarshal(created.Body.Bytes(), &receipt))
		require.NotEmpty(t, receipt.ID)
		assert.Equal(t, "queued", receipt.Status)
		assert.NotContains(t, created.Body.String(), "private-1")
		assert.NotContains(t, created.Body.String(), "channel-only-key")
		seen := <-uploads
		assert.Equal(t, "video-"+receipt.ID, seen.idempotency)
		assert.Equal(t, map[string][]string{"prompt": {"two distinct references"}, "ratio": {"16:9"}, "seconds": {"30"}}, seen.fields)
		assert.Equal(t, expectedImages, seen.images)
	}
	var task model.Task
	require.NoError(t, database.Where("task_id = ?", receipt.ID).First(&task).Error)
	assert.Equal(t, "private-1", task.PrivateData.UpstreamTaskID)
	assert.Equal(t, constant.TaskPlatform("custom-video"), task.Platform)
	assert.Equal(t, owner.Id, task.UserId)
	assert.Equal(t, ownerToken.Id, task.PrivateData.TokenId)
	assert.Equal(t, channel.Id, task.ChannelId)
	assert.Equal(t, customVideoModel, task.Properties.OriginModelName)
	assert.Equal(t, charge, task.Quota)
	require.NotNil(t, task.PrivateData.BillingContext)
	assert.Empty(t, task.PrivateData.BillingContext.OtherRatios)
	customVideoAssertQuota(t, database, owner.Id, ownerToken.Id, initialQuota-charge)

	for _, path := range []string{"/v1/videos/" + receipt.ID, "/v1/videos/" + receipt.ID + "/content", "/custom/v1/videos/" + receipt.ID} {
		response := customVideoRequest(engine, httptest.NewRequest(http.MethodGet, path, nil), otherToken.Key)
		missing := customVideoRequest(engine, httptest.NewRequest(http.MethodGet, strings.Replace(path, receipt.ID, "task_missing", 1), nil), otherToken.Key)
		assert.Contains(t, []int{http.StatusBadRequest, http.StatusNotFound}, response.Code)
		assert.Equal(t, missing.Code, response.Code, "foreign tasks must be indistinguishable from missing tasks")
		assert.JSONEq(t, missing.Body.String(), response.Body.String())
		response = customVideoRequest(engine, httptest.NewRequest(http.MethodGet, path, nil), "")
		assert.Equal(t, http.StatusUnauthorized, response.Code, response.Body.String())
	}
	assert.Zero(t, queryCalls.Load())
	assert.Zero(t, contentCalls.Load())
	service.RunTaskPollingOnce(context.Background(), nil)
	processing := customVideoRequest(engine, httptest.NewRequest(http.MethodGet, "/v1/videos/"+receipt.ID, nil), ownerToken.Key)
	require.Equal(t, http.StatusOK, processing.Code, processing.Body.String())
	assert.Contains(t, processing.Body.String(), `"status":"in_progress"`)
	assert.NotContains(t, processing.Body.String(), "private-1")
	status.Store("succeeded")
	service.RunTaskPollingOnce(context.Background(), nil)
	success := customVideoRequest(engine, httptest.NewRequest(http.MethodGet, "/v1/videos/"+receipt.ID, nil), ownerToken.Key)
	require.Equal(t, http.StatusOK, success.Code, success.Body.String())
	assert.Contains(t, success.Body.String(), `"status":"completed"`)
	assert.NotContains(t, success.Body.String(), media.URL)
	native := customVideoRequest(engine, httptest.NewRequest(http.MethodGet, "/custom/v1/videos/"+receipt.ID, nil), ownerToken.Key)
	require.Equal(t, http.StatusOK, native.Code, native.Body.String())
	assert.Contains(t, native.Body.String(), `"status":"succeeded"`)
	assert.Contains(t, native.Body.String(), receipt.ID)
	assert.NotContains(t, native.Body.String(), "private-1")
	content := customVideoRequest(engine, httptest.NewRequest(http.MethodGet, "/v1/videos/"+receipt.ID+"/content", nil), ownerToken.Key)
	require.Equal(t, http.StatusOK, content.Code, content.Body.String())
	assert.Equal(t, "video/mp4", content.Header().Get("Content-Type"))
	assert.Equal(t, videoBytes, content.Body.Bytes())
	customVideoAssertQuota(t, database, owner.Id, ownerToken.Id, initialQuota-charge)

	// The supplied plugin deliberately rejects repeated images[] references.
	repeated := customVideoRequest(engine, customVideoMultipartRequest(t, "images[]", "images[]"), ownerToken.Key)
	assert.Equal(t, http.StatusBadRequest, repeated.Code, repeated.Body.String())
	assert.Contains(t, repeated.Body.String(), "use image_1 through image_9")
	expectedSubmits := int32(1)
	if seeded {
		expectedSubmits = 0
	}
	assert.Equal(t, expectedSubmits, submitCalls.Load())

	if seeded {
		receipt.ID = seedCustomVideoTask(t, database, owner.Id, ownerToken, channel, "private-2", charge)
	} else {
		failedCreate := httptest.NewRequest(http.MethodPost, "/v1/videos", strings.NewReader(`{"model":"custom-video-30s","prompt":"will fail","ratio":"16:9","seconds":"30"}`))
		failedCreate.Header.Set("Content-Type", "application/json")
		failedReceipt := customVideoRequest(engine, failedCreate, ownerToken.Key)
		require.Equal(t, http.StatusOK, failedReceipt.Code, failedReceipt.Body.String())
		<-uploads
		require.NoError(t, common.Unmarshal(failedReceipt.Body.Bytes(), &receipt))
	}
	customVideoAssertQuota(t, database, owner.Id, ownerToken.Id, initialQuota-2*charge)
	status.Store("failed")
	service.RunTaskPollingOnce(context.Background(), nil)
	failed := customVideoRequest(engine, httptest.NewRequest(http.MethodGet, "/v1/videos/"+receipt.ID, nil), ownerToken.Key)
	require.Equal(t, http.StatusOK, failed.Code, failed.Body.String())
	assert.Contains(t, failed.Body.String(), `"status":"failed"`)
	failedNative := customVideoRequest(engine, httptest.NewRequest(http.MethodGet, "/custom/v1/videos/"+receipt.ID, nil), ownerToken.Key)
	require.Equal(t, http.StatusOK, failedNative.Code, failedNative.Body.String())
	assert.Contains(t, failedNative.Body.String(), "provider generation failed")
	customVideoAssertQuota(t, database, owner.Id, ownerToken.Id, initialQuota-charge)
	service.RunTaskPollingOnce(context.Background(), nil)
	customVideoAssertQuota(t, database, owner.Id, ownerToken.Id, initialQuota-charge)
	var refunds int64
	require.NoError(t, database.Model(&model.Log{}).Where("type = ? AND user_id = ?", model.LogTypeRefund, owner.Id).Count(&refunds).Error)
	assert.Equal(t, int64(1), refunds)
}

func seedCustomVideoTask(t *testing.T, database *gorm.DB, userID int, token model.Token, channel model.Channel, upstreamID string, charge int) string {
	t.Helper()
	now := time.Now().Unix()
	task := model.Task{
		TaskID: model.GenerateTaskID(), Platform: "custom-video", UserId: userID, Group: "default", ChannelId: channel.Id,
		Quota: charge, Action: "image_to_video", Status: model.TaskStatusSubmitted, Progress: "0%", SubmitTime: now, CreatedAt: now,
		Properties: model.Properties{OriginModelName: customVideoModel, UpstreamModelName: customVideoModel},
		PrivateData: model.TaskPrivateData{
			Key: channel.Key, UpstreamTaskID: upstreamID, TokenId: token.Id, BillingSource: service.BillingSourceWallet,
			BillingContext: &model.TaskBillingContext{ModelPrice: 0.25, GroupRatio: 2, OriginModelName: customVideoModel},
			Execution:      &model.TaskExecutionSnapshot{TaskPlugin: &model.TaskPluginSnapshot{Key: "custom-video", Version: "1.0.0", APIVersion: 1}},
		},
	}
	task.SetData(map[string]any{"code": "1", "task_id": upstreamID, "status": "queued"})
	require.NoError(t, database.Create(&task).Error)
	// Seed the already-paid snapshot directly; submission/pre-consumption is
	// covered by the submit lifecycle test rather than being simulated here.
	require.NoError(t, database.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]any{"quota": gorm.Expr("quota - ?", charge), "used_quota": gorm.Expr("used_quota + ?", charge)}).Error)
	require.NoError(t, database.Model(&model.Token{}).Where("id = ?", token.Id).Updates(map[string]any{"remain_quota": gorm.Expr("remain_quota - ?", charge), "used_quota": gorm.Expr("used_quota + ?", charge)}).Error)
	require.NoError(t, database.Model(&model.Channel{}).Where("id = ?", channel.Id).Update("used_quota", gorm.Expr("used_quota + ?", charge)).Error)
	return task.TaskID
}

func customVideoRequest(handler http.Handler, request *http.Request, key string) *httptest.ResponseRecorder {
	if key != "" {
		request.Header.Set("Authorization", "Bearer sk-"+key)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func customVideoMultipartRequest(t *testing.T, fields ...string) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for _, field := range [][2]string{{"model", customVideoModel}, {"prompt", "two distinct references"}, {"ratio", "16:9"}, {"seconds", "30"}} {
		require.NoError(t, writer.WriteField(field[0], field[1]))
	}
	files := [][2]string{{"first.png", "first-image"}, {"second.png", "second-image"}, {"third.png", "third-image"}}
	require.LessOrEqual(t, len(fields), len(files))
	for index, field := range fields {
		file := files[index]
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, file[0]))
		header.Set("Content-Type", "image/png")
		part, err := writer.CreatePart(header)
		require.NoError(t, err)
		_, err = io.WriteString(part, file[1])
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())
	request := httptest.NewRequest(http.MethodPost, "/v1/videos", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func customVideoAssertQuota(t *testing.T, db *gorm.DB, userID, tokenID, expected int) {
	t.Helper()
	var user model.User
	var token model.Token
	require.NoError(t, db.First(&user, userID).Error)
	require.NoError(t, db.First(&token, tokenID).Error)
	assert.Equal(t, expected, user.Quota, "wallet")
	assert.Equal(t, expected, token.RemainQuota, "token")
}
