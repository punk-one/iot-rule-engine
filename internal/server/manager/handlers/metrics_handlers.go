package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	pipelinesvc "github.com/punk-one/iot-rule-engine/internal/infra/pipeline"
	executorsvc "github.com/punk-one/iot-rule-engine/internal/server/executor"
)

// MetricsHandler 获取统计指标
func (m *Manager) MetricsHandler(c *gin.Context) {
	rules, _ := m.db.ListRules()
	resp := gin.H{
		"rules_count": len(rules),
	}
	if stats, err := executorsvc.StatsLocal(); err == nil {
		resp["executor"] = stats
	}
	if metrics, err := pipelinesvc.MetricsLocal(); err == nil {
		resp["pipeline"] = metrics
	}
	c.JSON(http.StatusOK, resp)
}
