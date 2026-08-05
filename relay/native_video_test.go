package relay

import (
	"testing"

	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNativeVideoAdapterRegistryKeepsLegacyTaskReadable(t *testing.T) {
	adaptor := GetNativeVideoAdaptor(constant.NativeVideoAdapterLingjing)
	require.NotNil(t, adaptor)
	assert.Equal(t, constant.NativeVideoAdapterLingjing, adaptor.AdapterKey())
	assert.NotNil(t, GetTaskAdaptor(constant.TaskPlatformLingjingVideo))
	assert.Nil(t, GetTaskAdaptor(constant.LegacyTaskPlatformLingjingVideo))
	assert.Nil(t, GetNativeVideoAdaptor("unknown-native-video"))

	legacyChannel := &model.Channel{Type: constant.ChannelTypeNativeVideo}
	assert.NotNil(t, GetNativeVideoAdaptorForTask(constant.LegacyTaskPlatformLingjingVideo, legacyChannel))
	assert.Nil(t, GetNativeVideoAdaptorForTask(constant.LegacyTaskPlatformLingjingVideo, &model.Channel{Type: constant.ChannelTypeOpenAI}))
}
