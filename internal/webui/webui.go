// Package webui serves the release-built management UI from the same origin as
// the API. No Node.js runtime or external web server is needed after packaging.
package webui

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed assets
var assets embed.FS

func Middleware() gin.HandlerFunc {
	root, _ := fs.Sub(assets, "assets")
	files := http.FileServer(http.FS(root))
	return func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/debug/") || (c.Request.Method != "GET" && c.Request.Method != "HEAD") {
			c.Next()
			return
		}
		name := strings.TrimPrefix(path.Clean(p), "/")
		if info, err := fs.Stat(root, name); err == nil && !info.IsDir() {
			c.Header("Cache-Control", "no-cache")
			files.ServeHTTP(c.Writer, c.Request)
		} else if path.Ext(name) == "" && strings.Contains(c.GetHeader("Accept"), "text/html") {
			body, _ := fs.ReadFile(root, "index.html")
			c.Header("Cache-Control", "no-store")
			c.Data(http.StatusOK, "text/html; charset=utf-8", body)
		} else if name == "." || name == "" {
			body, _ := fs.ReadFile(root, "index.html")
			c.Data(http.StatusOK, "text/html; charset=utf-8", body)
		} else {
			c.Status(http.StatusNotFound)
		}
		c.Abort()
	}
}
