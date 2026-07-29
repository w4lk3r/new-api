package constant

import "strings"

func IsChannelTypeAllowedForPath(channelType int, requestPath string) bool {
	isLingjingPath := strings.HasPrefix(requestPath, "/v1/lingjing/")
	if isLingjingPath {
		return channelType == ChannelTypeLingjingVideo
	}
	return channelType != ChannelTypeLingjingVideo
}
