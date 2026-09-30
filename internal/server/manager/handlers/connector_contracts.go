package handlers

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	executorsvc "github.com/punk-one/iot-rule-engine/internal/server/executor"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
)

type ConnectorDescriptor struct {
	Family        string   `json:"family"`
	Label         string   `json:"label,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty"`
	DisplayFields []string `json:"display_fields,omitempty"`
}

type ConnectorBindingSpec struct {
	Required             bool     `json:"required"`
	AcceptedFamilies     []string `json:"accepted_families,omitempty"`
	RequiredCapabilities []string `json:"required_capabilities,omitempty"`
}

type PluginDescriptorView struct {
	sdk.PluginInfo
	ConnectorDescriptor *ConnectorDescriptor  `json:"connector_descriptor,omitempty"`
	ConnectorBinding    *ConnectorBindingSpec `json:"connector_binding,omitempty"`
	Plane               string                `json:"plane,omitempty"`
}

type ConnectorCompatibility struct {
	Compatible bool     `json:"compatible"`
	Reasons    []string `json:"reasons,omitempty"`
}

type ConnectorResourceView struct {
	sdk.ConnectorResource
	// Connection shadows the embedded field so API responses never expose credentials.
	Connection        json.RawMessage          `json:"connection,omitempty"`
	PluginInfo        *PluginDescriptorView    `json:"plugin_info,omitempty"`
	ConnectorStatus   *sdk.ResourceStatusEvent `json:"status,omitempty"`
	ConnectionSummary string                   `json:"connection_summary,omitempty"`
	ConnectionPreview map[string]string        `json:"connection_preview,omitempty"`
	Compatibility     *ConnectorCompatibility  `json:"compatibility,omitempty"`
	RefCount          int                      `json:"ref_count,omitempty"`
	Pending           int                      `json:"pending,omitempty"`
	InflightWeight    int                      `json:"inflight_weight,omitempty"`
}

const connectorSecretMarker = "__IOT_RULE_SECRET_SET__"

func isConnectorSecretKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
	for _, token := range []string{"password", "passwd", "secret", "token", "api_key", "apikey", "access_key", "private_key", "credentials", "creds", "dsn"} {
		if normalized == token || strings.HasSuffix(normalized, "_"+token) {
			return true
		}
	}
	return false
}

func redactConnectorConnection(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return json.RawMessage("{}")
	}
	var redact func(interface{}) interface{}
	redact = func(input interface{}) interface{} {
		switch typed := input.(type) {
		case map[string]interface{}:
			out := make(map[string]interface{}, len(typed))
			for key, item := range typed {
				if isConnectorSecretKey(key) {
					out[key] = connectorSecretMarker
				} else {
					out[key] = redact(item)
				}
			}
			return out
		case []interface{}:
			out := make([]interface{}, len(typed))
			for i, item := range typed {
				out[i] = redact(item)
			}
			return out
		default:
			return input
		}
	}
	encoded, err := json.Marshal(redact(value))
	if err != nil {
		return json.RawMessage("{}")
	}
	return encoded
}

func mergeConnectorSecrets(incoming, stored json.RawMessage) json.RawMessage {
	if len(incoming) == 0 {
		return append(json.RawMessage(nil), stored...)
	}
	var next, previous map[string]interface{}
	if json.Unmarshal(incoming, &next) != nil || json.Unmarshal(stored, &previous) != nil {
		return incoming
	}
	var merge func(map[string]interface{}, map[string]interface{})
	merge = func(dst, old map[string]interface{}) {
		for key, oldValue := range old {
			newValue, exists := dst[key]
			if isConnectorSecretKey(key) {
				if !exists || newValue == "" || newValue == connectorSecretMarker {
					dst[key] = oldValue
				}
				continue
			}
			newMap, newOK := newValue.(map[string]interface{})
			oldMap, oldOK := oldValue.(map[string]interface{})
			if exists && newOK && oldOK {
				merge(newMap, oldMap)
			}
		}
	}
	merge(next, previous)
	encoded, err := json.Marshal(next)
	if err != nil {
		return incoming
	}
	return encoded
}

func (m *Manager) pluginDescriptorByIdentifier(identifier string) (*PluginDescriptorView, error) {
	pluginID, client, err := m.resolvePluginClient(identifier)
	if err == nil && client != nil {
		view := describePlugin(client.Info)
		return &view, nil
	}

	path, pathErr := m.resolvePluginPath(identifier)
	if pathErr != nil {
		if err != nil {
			return nil, err
		}
		return nil, pathErr
	}
	info, verifyErr := m.pluginLoader.VerifyPlugin(path)
	if verifyErr != nil {
		return nil, verifyErr
	}
	view := describePlugin(*info)
	if view.ID == "" {
		view.ID = pluginID
	}
	return &view, nil
}

func describePlugin(info sdk.PluginInfo) PluginDescriptorView {
	view := PluginDescriptorView{
		PluginInfo:          info,
		ConnectorDescriptor: inferConnectorDescriptor(info),
		ConnectorBinding:    inferConnectorBinding(info),
		Plane:               inferPluginPlane(info),
	}
	return view
}

func inferPluginPlane(info sdk.PluginInfo) string {
	if info.Type == sdk.PluginTypeConnector {
		return "connector"
	}

	id := normalizePluginID(info.ID)
	switch {
	case strings.Contains(id, "mqtt"), strings.Contains(id, "http"):
		return "edge"
	default:
		return "rule"
	}
}

func inferConnectorDescriptor(info sdk.PluginInfo) *ConnectorDescriptor {
	if info.Capabilities.ConnectorDescriptor != nil {
		return &ConnectorDescriptor{
			Family:        info.Capabilities.ConnectorDescriptor.Family,
			Label:         info.Capabilities.ConnectorDescriptor.Label,
			Capabilities:  append([]string(nil), info.Capabilities.ConnectorDescriptor.Capabilities...),
			DisplayFields: append([]string(nil), info.Capabilities.ConnectorDescriptor.DisplayFields...),
		}
	}
	if info.Type != sdk.PluginTypeConnector {
		return nil
	}

	id := normalizePluginID(info.ID)
	switch id {
	case "connect-s7":
		return &ConnectorDescriptor{
			Family:        "s7",
			Label:         "Siemens S7",
			Capabilities:  []string{"read", "write", "session:per_operation", "concurrency:serialized"},
			DisplayFields: []string{"host", "port", "rack", "slot"},
		}
	case "connect-timescaledb":
		return &ConnectorDescriptor{
			Family:        "timescaledb",
			Label:         "TimescaleDB / PostgreSQL",
			Capabilities:  []string{"read", "write", "shared_duplex", "connection_pool", "session:pooled"},
			DisplayFields: []string{"host", "port", "database"},
		}
	case "connect-postgres":
		return &ConnectorDescriptor{
			Family:        "postgres",
			Label:         "PostgreSQL",
			Capabilities:  []string{"read", "write", "shared_duplex", "connection_pool", "session:pooled"},
			DisplayFields: []string{"host", "port", "database"},
		}
	case "connect-mqtt":
		return &ConnectorDescriptor{
			Family:        "mqtt",
			Label:         "MQTT",
			Capabilities:  []string{"read", "write", "shared_duplex", "session:persistent"},
			DisplayFields: []string{"broker", "client_id"},
		}
	case "connect-redis":
		return &ConnectorDescriptor{
			Family:        "redis",
			Label:         "Redis",
			Capabilities:  []string{"read", "write", "shared_duplex", "session:pooled"},
			DisplayFields: []string{"addr", "db"},
		}
	case "connect-fanuc":
		return &ConnectorDescriptor{
			Family:        "fanuc",
			Label:         "Fanuc FOCAS",
			Capabilities:  []string{"read", "write", "session:per_operation", "concurrency:serialized"},
			DisplayFields: []string{"host", "port"},
		}
	case "connect-tdengine":
		return &ConnectorDescriptor{
			Family:        "tdengine",
			Label:         "TDengine",
			Capabilities:  []string{"read", "write", "connection_pool", "session:pooled"},
			DisplayFields: []string{"dsn", "database", "driver"},
		}
	default:
		family := strings.TrimPrefix(id, "connect-")
		if family == "" {
			family = id
		}
		return &ConnectorDescriptor{
			Family:       family,
			Capabilities: []string{"read", "write"},
		}
	}
}

func inferConnectorBinding(info sdk.PluginInfo) *ConnectorBindingSpec {
	if info.Capabilities.ConnectorBinding != nil {
		return &ConnectorBindingSpec{
			Required:             info.Capabilities.ConnectorBinding.Required,
			AcceptedFamilies:     append([]string(nil), info.Capabilities.ConnectorBinding.AcceptedFamilies...),
			RequiredCapabilities: append([]string(nil), info.Capabilities.ConnectorBinding.RequiredCapabilities...),
		}
	}
	id := normalizePluginID(info.ID)
	switch id {
	case "source-s7":
		return &ConnectorBindingSpec{
			Required:             true,
			AcceptedFamilies:     []string{"s7"},
			RequiredCapabilities: []string{"read"},
		}
	case "sink-s7":
		return &ConnectorBindingSpec{
			Required:             true,
			AcceptedFamilies:     []string{"s7"},
			RequiredCapabilities: []string{"write"},
		}
	case "sink-timeseries-timescaledb":
		return &ConnectorBindingSpec{
			Required:             true,
			AcceptedFamilies:     []string{"timescaledb"},
			RequiredCapabilities: []string{"write"},
		}
	case "source-timeseries-timescaledb":
		return &ConnectorBindingSpec{
			Required:             true,
			AcceptedFamilies:     []string{"timescaledb"},
			RequiredCapabilities: []string{"read"},
		}
	case "source-relational-postgres":
		return &ConnectorBindingSpec{
			Required:             true,
			AcceptedFamilies:     []string{"postgres"},
			RequiredCapabilities: []string{"read"},
		}
	case "sink-relational-postgres":
		return &ConnectorBindingSpec{
			Required:             true,
			AcceptedFamilies:     []string{"postgres"},
			RequiredCapabilities: []string{"write"},
		}
	case "source-timeseries-tdengine":
		return &ConnectorBindingSpec{
			Required:             true,
			AcceptedFamilies:     []string{"tdengine"},
			RequiredCapabilities: []string{"read"},
		}
	case "sink-timeseries-tdengine":
		return &ConnectorBindingSpec{
			Required:             true,
			AcceptedFamilies:     []string{"tdengine"},
			RequiredCapabilities: []string{"write"},
		}
	case "source-mqtt":
		return &ConnectorBindingSpec{
			Required:             true,
			AcceptedFamilies:     []string{"mqtt"},
			RequiredCapabilities: []string{"read"},
		}
	case "sink-mqtt":
		return &ConnectorBindingSpec{
			Required:             true,
			AcceptedFamilies:     []string{"mqtt"},
			RequiredCapabilities: []string{"write"},
		}
	case "source-redis":
		return &ConnectorBindingSpec{
			Required:             true,
			AcceptedFamilies:     []string{"redis"},
			RequiredCapabilities: []string{"read"},
		}
	case "sink-redis":
		return &ConnectorBindingSpec{
			Required:             true,
			AcceptedFamilies:     []string{"redis"},
			RequiredCapabilities: []string{"write"},
		}
	case "source-fanuc":
		return &ConnectorBindingSpec{
			Required:             true,
			AcceptedFamilies:     []string{"fanuc"},
			RequiredCapabilities: []string{"read"},
		}
	case "sink-fanuc":
		return &ConnectorBindingSpec{
			Required:             true,
			AcceptedFamilies:     []string{"fanuc"},
			RequiredCapabilities: []string{"write"},
		}
	default:
		return nil
	}
}

func normalizePluginID(id string) string {
	id = strings.TrimSpace(strings.ToLower(id))
	id = strings.ReplaceAll(id, "_", "-")
	return id
}

func compatibilityFor(binding *ConnectorBindingSpec, descriptor *ConnectorDescriptor) *ConnectorCompatibility {
	if binding == nil {
		return nil
	}

	reasons := make([]string, 0, 2)
	if descriptor == nil {
		reasons = append(reasons, "connector plugin has no descriptor")
		return &ConnectorCompatibility{Compatible: false, Reasons: reasons}
	}

	if len(binding.AcceptedFamilies) > 0 && !containsString(binding.AcceptedFamilies, descriptor.Family) {
		reasons = append(reasons, fmt.Sprintf("family %s is not accepted", descriptor.Family))
	}
	for _, capability := range binding.RequiredCapabilities {
		if !containsString(descriptor.Capabilities, capability) {
			reasons = append(reasons, fmt.Sprintf("capability %s is required", capability))
		}
	}

	return &ConnectorCompatibility{
		Compatible: len(reasons) == 0,
		Reasons:    reasons,
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func parseConnectionPreview(resource sdk.ConnectorResource, descriptor *ConnectorDescriptor) map[string]string {
	if descriptor == nil || len(resource.Connection) == 0 {
		return nil
	}

	preview := map[string]string{}
	switch descriptor.Family {
	case "s7":
		var cfg struct {
			Host string `json:"host"`
			Port int    `json:"port"`
			Rack int    `json:"rack"`
			Slot int    `json:"slot"`
		}
		if err := json.Unmarshal(resource.Connection, &cfg); err == nil {
			if cfg.Host != "" {
				preview["host"] = cfg.Host
			}
			if cfg.Port > 0 {
				preview["port"] = strconv.Itoa(cfg.Port)
			}
			preview["rack"] = strconv.Itoa(cfg.Rack)
			preview["slot"] = strconv.Itoa(cfg.Slot)
		}
	case "timescaledb", "postgres":
		var cfg struct {
			DSN string `json:"dsn"`
		}
		if err := json.Unmarshal(resource.Connection, &cfg); err == nil && strings.TrimSpace(cfg.DSN) != "" {
			for key, value := range parseDatabaseDSN(cfg.DSN) {
				preview[key] = value
			}
		}
	default:
		var raw map[string]interface{}
		if err := json.Unmarshal(resource.Connection, &raw); err == nil {
			for _, field := range descriptor.DisplayFields {
				if value, ok := raw[field]; ok {
					preview[field] = fmt.Sprint(value)
				}
			}
		}
	}

	if len(preview) == 0 {
		return nil
	}
	return preview
}

func parseDatabaseDSN(dsn string) map[string]string {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil
	}

	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		if parsed, err := url.Parse(dsn); err == nil {
			result := map[string]string{}
			if parsed.Hostname() != "" {
				result["host"] = parsed.Hostname()
			}
			if parsed.Port() != "" {
				result["port"] = parsed.Port()
			}
			dbName := strings.TrimPrefix(parsed.Path, "/")
			if dbName != "" {
				result["database"] = dbName
			}
			return result
		}
	}

	result := map[string]string{}
	for _, token := range strings.Fields(dsn) {
		parts := strings.SplitN(token, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
		switch key {
		case "host", "port":
			result[key] = value
		case "dbname", "database":
			result["database"] = value
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func buildConnectionSummary(preview map[string]string, descriptor *ConnectorDescriptor, resource sdk.ConnectorResource) string {
	if descriptor == nil {
		return ""
	}

	switch descriptor.Family {
	case "s7":
		host := preview["host"]
		port := preview["port"]
		if host == "" {
			return resource.PluginID
		}
		parts := []string{host}
		if port != "" {
			parts[0] = host + ":" + port
		}
		if rack, ok := preview["rack"]; ok {
			parts = append(parts, "rack="+rack)
		}
		if slot, ok := preview["slot"]; ok {
			parts = append(parts, "slot="+slot)
		}
		return strings.Join(parts, " ")
	case "timescaledb", "postgres":
		host := preview["host"]
		port := preview["port"]
		dbName := preview["database"]
		switch {
		case host != "" && port != "" && dbName != "":
			return fmt.Sprintf("%s:%s/%s", host, port, dbName)
		case host != "" && dbName != "":
			return fmt.Sprintf("%s/%s", host, dbName)
		case dbName != "":
			return dbName
		default:
			return "DSN configured"
		}
	default:
		if len(preview) == 0 {
			return resource.PluginID
		}
		parts := make([]string, 0, len(preview))
		keys := make([]string, 0, len(preview))
		for key := range preview {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			parts = append(parts, key+"="+preview[key])
		}
		return strings.Join(parts, " ")
	}
}

func (m *Manager) buildConnectorResourceView(resource *sdk.ConnectorResource, binding *ConnectorBindingSpec) ConnectorResourceView {
	view := ConnectorResourceView{}
	if resource == nil {
		return view
	}
	view.ConnectorResource = *resource
	view.Connection = redactConnectorConnection(resource.Connection)

	if pluginInfo, err := m.pluginDescriptorByIdentifier(resource.PluginID); err == nil {
		view.PluginInfo = pluginInfo
		preview := parseConnectionPreview(*resource, pluginInfo.ConnectorDescriptor)
		view.ConnectionPreview = preview
		view.ConnectionSummary = buildConnectionSummary(preview, pluginInfo.ConnectorDescriptor, *resource)
		view.Compatibility = compatibilityFor(binding, pluginInfo.ConnectorDescriptor)
	}

	if snapshot, ok := executorsvc.GetConnectorSnapshotLocal(resource.ID); ok {
		status := snapshot.Status
		view.ConnectorStatus = &status
		view.RefCount = snapshot.RefCount
		view.Pending = snapshot.Pending
		view.InflightWeight = snapshot.InflightWeight
	} else {
		status := sdk.ResourceStatusEvent{
			ResourceID:       resource.ID,
			ProviderPluginID: resource.PluginID,
			Status:           sdk.ResourceStatusUnknown,
		}
		view.ConnectorStatus = &status
	}

	return view
}

func (m *Manager) listCompatibleConnectorViews(binding *ConnectorBindingSpec) ([]ConnectorResourceView, error) {
	if m.db == nil {
		return nil, fmt.Errorf("rules database is not available")
	}

	resources, err := m.db.ListConnectorResources()
	if err != nil {
		return nil, err
	}

	views := make([]ConnectorResourceView, 0, len(resources))
	for _, resource := range resources {
		if resource == nil {
			continue
		}
		view := m.buildConnectorResourceView(resource, binding)
		if binding == nil || (view.Compatibility != nil && view.Compatibility.Compatible) {
			views = append(views, view)
		}
	}

	sort.Slice(views, func(i, j int) bool {
		if views[i].Name != views[j].Name {
			return views[i].Name < views[j].Name
		}
		return views[i].ID < views[j].ID
	})
	return views, nil
}

func (m *Manager) ListCompatibleConnectorsHandler(c *gin.Context) {
	pluginInfo, err := m.pluginDescriptorByIdentifier(c.Param("name"))
	if err != nil {
		c.JSON(404, gin.H{"error": err.Error()})
		return
	}

	views, err := m.listCompatibleConnectorViews(pluginInfo.ConnectorBinding)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"plugin":     pluginInfo,
		"connectors": views,
		"count":      len(views),
	})
}
