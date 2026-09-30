package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
)

// Gray Release API Handlers

// CreateGrayReleaseConfigHandler 创建灰度发布配置
func (m *Manager) CreateGrayReleaseConfigHandler(c *gin.Context) {
	ruleID := c.Param("id")

	var config schema.GrayReleaseConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 设置规则ID
	config.RuleID = ruleID

	// 验证配置
	if err := validateGrayReleaseConfig(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 保存配置
	if err := m.db.SaveGrayReleaseConfig(&config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, config)
}

// GetGrayReleaseConfigHandler 获取灰度发布配置
func (m *Manager) GetGrayReleaseConfigHandler(c *gin.Context) {
	ruleID := c.Param("id")

	config, err := m.db.GetGrayReleaseConfig(ruleID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, config)
}

// UpdateGrayReleaseConfigHandler 更新灰度发布配置
func (m *Manager) UpdateGrayReleaseConfigHandler(c *gin.Context) {
	ruleID := c.Param("id")

	var config schema.GrayReleaseConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 设置规则ID
	config.RuleID = ruleID

	// 验证配置
	if err := validateGrayReleaseConfig(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 保存配置
	if err := m.db.SaveGrayReleaseConfig(&config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, config)
}

// DeleteGrayReleaseConfigHandler 删除灰度发布配置
func (m *Manager) DeleteGrayReleaseConfigHandler(c *gin.Context) {
	ruleID := c.Param("id")

	if err := m.db.DeleteGrayReleaseConfig(ruleID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "gray release config deleted"})
}

// validateGrayReleaseConfig 验证灰度发布配置
func validateGrayReleaseConfig(config *schema.GrayReleaseConfig) error {
	if config.Strategy != "percentage" && config.Strategy != "condition" {
		return &ValidationConfigError{Message: "strategy must be 'percentage' or 'condition'"}
	}

	if config.Strategy == "percentage" && (config.Percentage < 0 || config.Percentage > 100) {
		return &ValidationConfigError{Message: "percentage must be between 0 and 100"}
	}

	if len(config.TargetNodes) == 0 {
		return &ValidationConfigError{Message: "target_nodes cannot be empty"}
	}

	return nil
}

// ValidationConfigError 配置验证错误
type ValidationConfigError struct {
	Message string
}

func (e *ValidationConfigError) Error() string {
	return e.Message
}
