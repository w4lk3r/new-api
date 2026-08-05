package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateNativeVideoChannelAdaptersAndTasks(t *testing.T) {
	truncateTables(t)

	legacyNativeChannel := &Channel{
		Type:   constant.ChannelTypeNativeVideo,
		Key:    "native-key",
		Status: common.ChannelStatusEnabled,
		Name:   "legacy-native-video",
		Models: "lingjing-video-v1",
		Group:  "default",
	}
	unrelatedChannel := &Channel{
		Type:   constant.ChannelTypeOpenAI,
		Key:    "other-key",
		Status: common.ChannelStatusEnabled,
		Name:   "unrelated-channel",
		Models: "gpt-4o-mini",
		Group:  "default",
	}
	require.NoError(t, DB.Create(legacyNativeChannel).Error)
	require.NoError(t, DB.Create(unrelatedChannel).Error)

	legacyNativeTask := &Task{
		TaskID:    "task_native_legacy",
		Platform:  constant.LegacyTaskPlatformLingjingVideo,
		ChannelId: legacyNativeChannel.Id,
		Status:    TaskStatusInProgress,
	}
	unrelatedLegacyTask := &Task{
		TaskID:    "task_unrelated_legacy",
		Platform:  constant.LegacyTaskPlatformLingjingVideo,
		ChannelId: unrelatedChannel.Id,
		Status:    TaskStatusInProgress,
	}
	require.NoError(t, DB.Create(legacyNativeTask).Error)
	require.NoError(t, DB.Create(unrelatedLegacyTask).Error)

	require.NoError(t, migrateNativeVideoChannelAdaptersAndTasks())

	var migratedChannel Channel
	require.NoError(t, DB.First(&migratedChannel, legacyNativeChannel.Id).Error)
	assert.Equal(t, constant.NativeVideoAdapterLingjing, migratedChannel.AdapterKey)

	var migratedTask Task
	require.NoError(t, DB.First(&migratedTask, legacyNativeTask.ID).Error)
	assert.Equal(t, constant.TaskPlatformLingjingVideo, migratedTask.Platform)

	var untouchedTask Task
	require.NoError(t, DB.First(&untouchedTask, unrelatedLegacyTask.ID).Error)
	assert.Equal(t, constant.LegacyTaskPlatformLingjingVideo, untouchedTask.Platform)
}
