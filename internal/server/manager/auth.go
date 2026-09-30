package manager

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

// AuthMiddleware Token 鉴权中间件
func AuthMiddleware(token string, enabled bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 如果未启用鉴权，直接通过
		if !enabled || token == "" {
			c.Next()
			return
		}

		// 从 Header 获取 Token
		authHeader := c.GetHeader("Authorization")
		// 移除 "Bearer " 前缀（如果有）
		providedToken := strings.TrimPrefix(authHeader, "Bearer ")
		providedToken = strings.TrimSpace(providedToken)

		// 验证 Token
		if providedToken != token {
			pkglog.Warn("Manager API: unauthorized request from %s", c.ClientIP())
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "unauthorized",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

func extractBearerToken(c *gin.Context) string {
	authHeader := c.GetHeader("Authorization")
	providedToken := strings.TrimPrefix(authHeader, "Bearer ")
	return strings.TrimSpace(providedToken)
}
