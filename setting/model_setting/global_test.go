package model_setting

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveNativeVideoModel(t *testing.T) {
	original := globalSettings
	globalSettings = GlobalSettings{
		NativeVideoDefaultModel: "video-standard-15s",
		NativeVideoGroupDefaultModels: map[string]string{
			"premium": "video-premium-15s",
		},
	}
	t.Cleanup(func() {
		globalSettings = original
	})

	assert.Equal(t, "video-explicit-15s", ResolveNativeVideoModel("premium", "video-explicit-15s"))
	assert.Equal(t, "video-premium-15s", ResolveNativeVideoModel("premium", "video-default"))
	assert.Equal(t, "video-premium-15s", ResolveNativeVideoModel("premium", ""))
	assert.Equal(t, "video-standard-15s", ResolveNativeVideoModel("default", "video-default"))
}
