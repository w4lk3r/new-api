package constant

import "strings"

func IsNativeVideoPath(requestPath string) bool {
	return strings.HasPrefix(requestPath, "/v1/lingjing/") ||
		strings.HasPrefix(requestPath, "/v1/native-video/")
}

// NativeVideoAdapterKeyForPath returns the platform required by a
// provider-specific native route. The model-driven native-video route has no
// fixed adapter and is selected by model, group, priority, and capability.
func NativeVideoAdapterKeyForPath(requestPath string) string {
	if strings.HasPrefix(requestPath, "/v1/lingjing/") {
		return NativeVideoAdapterLingjing
	}
	return ""
}

func IsNativeVideoCreatePath(requestPath string) bool {
	return requestPath == "/v1/lingjing/videos" || requestPath == "/v1/native-video/generations"
}

func IsChannelTypeAllowedForPath(channelType int, requestPath string) bool {
	if IsNativeVideoPath(requestPath) {
		return channelType == ChannelTypeNativeVideo
	}
	return channelType != ChannelTypeNativeVideo
}
