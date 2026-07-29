package constant

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLingjingChannelPathIsolation(t *testing.T) {
	assert.True(t, IsChannelTypeAllowedForPath(ChannelTypeLingjingVideo, "/v1/lingjing/videos"))
	assert.False(t, IsChannelTypeAllowedForPath(ChannelTypeOpenAI, "/v1/lingjing/videos"))
	assert.False(t, IsChannelTypeAllowedForPath(ChannelTypeLingjingVideo, "/v1/videos"))
	assert.True(t, IsChannelTypeAllowedForPath(ChannelTypeOpenAI, "/v1/videos"))
}
