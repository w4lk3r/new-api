package controller

import "github.com/gin-gonic/gin"

// Lingjing route handlers remain as compatibility aliases. The generic
// NativeVideoService resolves the protocol through adapter_key and never
// imports a concrete provider from its controller layer.
func LingjingVideoUpload(c *gin.Context) {
	NativeVideoUpload(c)
}

func LingjingVideoFetch(c *gin.Context) {
	NativeVideoFetch(c)
}

func LingjingVideoDownload(c *gin.Context) {
	NativeVideoDownload(c)
}
