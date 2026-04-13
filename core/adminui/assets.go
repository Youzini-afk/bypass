package adminui

import (
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed dist/*
var embeddedAssets embed.FS

func Mount(engine *gin.Engine) {
	engine.GET("/", func(ctx *gin.Context) {
		serveFile(ctx, "index.html")
	})
	engine.GET("/assets/*filepath", func(ctx *gin.Context) {
		filepath := strings.TrimPrefix(ctx.Param("filepath"), "/")
		if filepath == "" {
			ctx.Status(http.StatusNotFound)
			return
		}
		serveFile(ctx, path.Join("assets", filepath))
	})
	engine.GET("/favicon.ico", func(ctx *gin.Context) {
		serveFile(ctx, "favicon.ico")
	})
}

func serveFile(ctx *gin.Context, name string) {
	root, err := fs.Sub(embeddedAssets, "dist")
	if err != nil {
		ctx.Status(http.StatusInternalServerError)
		return
	}

	buffer, err := fs.ReadFile(root, name)
	if err != nil {
		ctx.Status(http.StatusNotFound)
		return
	}

	contentType := mime.TypeByExtension(path.Ext(name))
	if contentType == "" {
		contentType = http.DetectContentType(buffer)
	}
	ctx.Data(http.StatusOK, contentType, buffer)
}
