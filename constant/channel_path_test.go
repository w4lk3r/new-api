package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNativeVideoChannelPathIsolation(t *testing.T) {
	assert.True(t, IsChannelTypeAllowedForPath(ChannelTypeNativeVideo, "/v1/lingjing/videos"))
	assert.False(t, IsChannelTypeAllowedForPath(ChannelTypeOpenAI, "/v1/lingjing/videos"))
	assert.True(t, IsChannelTypeAllowedForPath(ChannelTypeNativeVideo, "/v1/native-video/generations"))
	assert.False(t, IsChannelTypeAllowedForPath(ChannelTypeOpenAI, "/v1/native-video/generations"))
	assert.False(t, IsChannelTypeAllowedForPath(ChannelTypeNativeVideo, "/v1/videos"))
	assert.True(t, IsChannelTypeAllowedForPath(ChannelTypeOpenAI, "/v1/videos"))
}

func TestNativeVideoAdapterKeyForPath(t *testing.T) {
	assert.Equal(t, NativeVideoAdapterLingjing, NativeVideoAdapterKeyForPath("/v1/lingjing/videos"))
	assert.Empty(t, NativeVideoAdapterKeyForPath("/v1/native-video/generations"))
	assert.True(t, IsNativeVideoCreatePath("/v1/native-video/generations"))
}
