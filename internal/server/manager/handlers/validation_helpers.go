package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"github.com/punk-one/iot-rule-plugin-sdk/schema"
	"github.com/punk-one/iot-rule-engine/internal/core/engine"
	pkglog "github.com/punk-one/iot-rule-engine/pkg/log"
)

var deployNodeErrorPattern = regexp.MustCompile(`failed to (?:create|init) plugin for node ([^:]+): (.+)$`)

// validateNodeConfig 验证节点配置（基于 JSON Schema）
func (m *Manager) validateNodeConfig(node *schema.NodeConfig) error {
	// 确定插件 ID
	pluginID := node.Type
	if node.Plugin != "" {
		pluginID = node.Plugin
	}

	// 获取插件信息
	pluginClient, err := m.pluginLoader.GetPluginClient(pluginID)
	if err != nil {
		// 插件未加载，返回错误
		return fmt.Errorf("plugin %s is not loaded: %w", pluginID, err)
	}

	// 使用插件的 ConfigSchema 验证配置
	configSchema := pluginClient.Info.Capabilities.ConfigSchema
	if configSchema == "" {
		return m.validateConnectorReference(node, pluginClient.Info)
	}

	// 解析 Schema
	if err := validatePayloadAgainstSchema(node.Config, configSchema, pluginID); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	// connector_ref 现在必须引用一个已存在的 ConnectorResource。
	return m.validateConnectorReference(node, pluginClient.Info)
}

func (m *Manager) validateConnectorReference(node *schema.NodeConfig, pluginInfo sdk.PluginInfo) error {
	if node == nil || node.Config == nil || m.db == nil {
		return nil
	}

	binding := inferConnectorBinding(pluginInfo)
	rawRef, ok := node.Config["connector_ref"]
	if !ok {
		if binding != nil && binding.Required {
			return fmt.Errorf("connector_ref is required")
		}
		return nil
	}
	ref, ok := rawRef.(string)
	if !ok || strings.TrimSpace(ref) == "" {
		return fmt.Errorf("connector_ref must be a non-empty string")
	}
	resource, err := m.db.GetConnectorResource(ref)
	if err != nil {
		return fmt.Errorf("connector_ref %s not found", ref)
	}

	if binding == nil {
		return nil
	}

	descriptor, err := m.pluginDescriptorByIdentifier(resource.PluginID)
	if err != nil {
		return fmt.Errorf("failed to resolve connector plugin %s: %w", resource.PluginID, err)
	}
	compatibility := compatibilityFor(binding, descriptor.ConnectorDescriptor)
	if compatibility != nil && !compatibility.Compatible {
		return fmt.Errorf("connector_ref %s is incompatible with plugin %s: %s", ref, pluginInfo.ID, strings.Join(compatibility.Reasons, "; "))
	}
	return nil
}

func (m *Manager) validateConnectorResourceConnection(info sdk.PluginInfo, raw json.RawMessage) error {
	if strings.TrimSpace(info.Capabilities.ConfigSchema) == "" || len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return fmt.Errorf("connection must be a JSON object: %w", err)
	}

	if err := validatePayloadAgainstSchema(payload, info.Capabilities.ConfigSchema, info.ID); err != nil {
		return fmt.Errorf("connection validation failed: %w", err)
	}
	return nil
}

func validatePayloadAgainstSchema(payload map[string]interface{}, rawSchema string, pluginID string) error {
	var schemaMap map[string]interface{}
	if err := json.Unmarshal([]byte(rawSchema), &schemaMap); err != nil {
		pkglog.Warn("Failed to parse config schema for plugin %s: %v", pluginID, err)
		return nil
	}
	return validateNestedConfig(payload, schemaMap)
}

// validateRuleDeployment 在真正写库/同步前做一次 DAG 预检，避免无效配置进入部署链路。
func (m *Manager) validateRuleDeployment(rule *schema.RuleFlow) []ValidationError {
	dag, err := engine.BuildDAGWithLoader(rule, m.pluginLoader)
	if err != nil {
		return validationErrorsFromRuntimeError(err, "INVALID_DEPLOYMENT")
	}
	stopPreflightPlugins(dag)
	return nil
}

func stopPreflightPlugins(dag *engine.DAG) {
	if dag == nil {
		return
	}
	for nodeID, plugin := range dag.Plugins {
		if plugin == nil {
			continue
		}
		if err := plugin.Stop(); err != nil {
			pkglog.Warn("Failed to stop preflight plugin for node %s: %v", nodeID, err)
		}
	}
}

func validationErrorsFromRuntimeError(err error, defaultCode string) []ValidationError {
	if err == nil {
		return nil
	}

	if details := extractValidationErrorsFromEnvelope(err.Error(), defaultCode); len(details) > 0 {
		return details
	}

	return validationErrorsFromMessage(err.Error(), defaultCode)
}

func extractValidationErrorsFromEnvelope(raw string, defaultCode string) []ValidationError {
	start := strings.Index(raw, "{")
	if start < 0 {
		return nil
	}

	var payload struct {
		Error   string          `json:"error"`
		Message string          `json:"message"`
		Details json.RawMessage `json:"details"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw[start:])), &payload); err != nil {
		return nil
	}

	if details := decodeValidationErrors(payload.Details, defaultCode); len(details) > 0 {
		return details
	}
	if payload.Error != "" {
		return validationErrorsFromMessage(payload.Error, defaultCode)
	}
	if payload.Message != "" {
		return validationErrorsFromMessage(payload.Message, defaultCode)
	}
	return nil
}

func decodeValidationErrors(raw json.RawMessage, defaultCode string) []ValidationError {
	if len(raw) == 0 {
		return nil
	}

	var details []ValidationError
	if err := json.Unmarshal(raw, &details); err == nil && len(details) > 0 {
		for i := range details {
			if details[i].Code == "" {
				details[i].Code = defaultCode
			}
		}
		return details
	}

	var detailText string
	if err := json.Unmarshal(raw, &detailText); err == nil && detailText != "" {
		return validationErrorsFromMessage(detailText, defaultCode)
	}

	var detailObj map[string]interface{}
	if err := json.Unmarshal(raw, &detailObj); err == nil {
		if msg, ok := detailObj["message"].(string); ok && msg != "" {
			return validationErrorsFromMessage(msg, defaultCode)
		}
		if msg, ok := detailObj["error"].(string); ok && msg != "" {
			return validationErrorsFromMessage(msg, defaultCode)
		}
	}

	return nil
}

func validationErrorsFromMessage(message string, defaultCode string) []ValidationError {
	trimmed := strings.TrimSpace(message)
	if trimmed == "" {
		return []ValidationError{{
			Message: "unknown error",
			Code:    defaultCode,
		}}
	}

	for _, prefix := range []string{
		"failed to build DAG: ",
		"DAG validation failed: ",
	} {
		trimmed = strings.TrimPrefix(trimmed, prefix)
	}

	if matches := deployNodeErrorPattern.FindStringSubmatch(trimmed); len(matches) == 3 {
		detail := ValidationError{
			NodeID:  matches[1],
			Message: strings.TrimSpace(matches[2]),
			Code:    defaultCode,
		}
		if field := inferErrorField(detail.Message); field != "" {
			detail.Field = field
		}
		return []ValidationError{detail}
	}

	return []ValidationError{{
		Message: trimmed,
		Code:    defaultCode,
	}}
}

func inferErrorField(message string) string {
	for _, separator := range []string{" must ", " should ", " cannot ", " is required"} {
		if idx := strings.Index(message, separator); idx > 0 {
			candidate := strings.TrimSpace(message[:idx])
			if strings.Contains(candidate, ".") {
				return candidate
			}
		}
	}
	return ""
}

// hasField 检查配置中是否存在字段（支持点号路径）
func hasField(config map[string]interface{}, path string) bool {
	parts := splitPath(path)
	current := interface{}(config)

	for _, part := range parts {
		if currentMap, ok := current.(map[string]interface{}); ok {
			if val, exists := currentMap[part]; exists {
				current = val
			} else {
				return false
			}
		} else {
			return false
		}
	}

	return true
}

// splitPath 分割路径（支持点号分隔）
func splitPath(path string) []string {
	parts := []string{}
	current := ""

	for _, char := range path {
		if char == '.' {
			if current != "" {
				parts = append(parts, current)
				current = ""
			}
		} else {
			current += string(char)
		}
	}

	if current != "" {
		parts = append(parts, current)
	}

	return parts
}

// validateNestedConfig 验证嵌套配置结构
func validateNestedConfig(config map[string]interface{}, schemaMap map[string]interface{}) error {
	// 检查 additionalProperties: false
	if additionalProps, ok := schemaMap["additionalProperties"]; ok {
		if additionalProps == false {
			// 只允许 schema 中定义的属性
			properties, ok := schemaMap["properties"].(map[string]interface{})
			if ok {
				for key := range config {
					if _, exists := properties[key]; !exists {
						return fmt.Errorf("unknown field '%s' (additionalProperties is false)", key)
					}
				}
			}
		}
	}

	// 验证当前层级的 required 字段
	if required, ok := schemaMap["required"].([]interface{}); ok {
		for _, req := range required {
			if reqStr, ok := req.(string); ok {
				if !hasField(config, reqStr) {
					return fmt.Errorf("required field '%s' is missing", reqStr)
				}
			}
		}
	}

	// 递归验证嵌套对象
	properties, ok := schemaMap["properties"].(map[string]interface{})
	if !ok {
		return nil
	}

	for key, prop := range properties {
		if propMap, ok := prop.(map[string]interface{}); ok {
			if propMap["type"] == "object" {
				if configValue, exists := config[key]; exists {
					if configMap, ok := configValue.(map[string]interface{}); ok {
						if err := validateNestedConfig(configMap, propMap); err != nil {
							return fmt.Errorf("%s.%w", key, err)
						}
					}
				}
			}
		}
	}

	return nil
}
