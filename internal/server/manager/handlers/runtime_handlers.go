package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/punk-one/iot-rule-engine/internal/core/dlq"
	"github.com/punk-one/iot-rule-engine/internal/core/runtime"
	"github.com/punk-one/iot-rule-engine/internal/infra/bus"
	executorsvc "github.com/punk-one/iot-rule-engine/internal/server/executor"
)

type RuntimeHandlers struct {
	runtimeMgr  *runtime.Manager
	manager     *Manager
	executorURL string
}

func NewRuntimeHandlers(runtimeMgr *runtime.Manager, manager *Manager, executorURL string) *RuntimeHandlers {
	return &RuntimeHandlers{
		runtimeMgr:  runtimeMgr,
		manager:     manager,
		executorURL: executorURL,
	}
}

func (h *RuntimeHandlers) proxyToExecutor(c *gin.Context) {
	if h.executorURL == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "executor service not available"})
		return
	}

	path := c.Request.URL.Path
	if len(path) >= 7 && path[:7] == "/api/v1" {
		path = path[7:]
	}

	url := h.executorURL + path
	if c.Request.URL.RawQuery != "" {
		url += "?" + c.Request.URL.RawQuery
	}

	req, err := http.NewRequest(c.Request.Method, url, c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	for key, values := range c.Request.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}

	for key, values := range resp.Header {
		for _, value := range values {
			c.Header(key, value)
		}
	}

	c.Data(resp.StatusCode, resp.Header.Get("Content-Type"), body)
}

func (h *RuntimeHandlers) useLocalRuntime() bool {
	return h.executorURL == "" && h.runtimeMgr != nil
}

func (h *RuntimeHandlers) ListRuleInstancesHandler(c *gin.Context) {
	instances, err := h.fetchRuleInstances()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{
			"error":       "failed to fetch runtime instances",
			"details":     err.Error(),
			"executorURL": h.executorURL,
		})
		return
	}

	latestInstances := make(map[string]*runtime.RuleInstance)
	for _, instance := range instances {
		existing, exists := latestInstances[instance.RuleID]
		if !exists {
			latestInstances[instance.RuleID] = instance
			continue
		}

		if instance.Status == runtime.RuleStatusRunning && existing.Status != runtime.RuleStatusRunning {
			latestInstances[instance.RuleID] = instance
			continue
		}
		if existing.Status == runtime.RuleStatusRunning && instance.Status != runtime.RuleStatusRunning {
			continue
		}

		v1, _ := strconv.Atoi(instance.Version)
		v2, _ := strconv.Atoi(existing.Version)
		if v1 > v2 {
			latestInstances[instance.RuleID] = instance
			continue
		}
		if v1 == v2 && instance.CreatedAt.After(existing.CreatedAt) {
			latestInstances[instance.RuleID] = instance
		}
	}

	instancesWithName := make([]map[string]interface{}, 0, len(latestInstances))
	for _, instance := range latestInstances {
		if instance.Status == runtime.RuleStatusStopped {
			continue
		}
		instancesWithName = append(instancesWithName, h.instanceResponse(instance))
	}

	sort.Slice(instancesWithName, func(i, j int) bool {
		leftName := strings.ToLower(fmt.Sprint(instancesWithName[i]["name"]))
		rightName := strings.ToLower(fmt.Sprint(instancesWithName[j]["name"]))
		if leftName == rightName {
			return fmt.Sprint(instancesWithName[i]["rule_id"]) < fmt.Sprint(instancesWithName[j]["rule_id"])
		}
		return leftName < rightName
	})

	c.JSON(http.StatusOK, gin.H{
		"instances": instancesWithName,
		"count":     len(instancesWithName),
	})
}

func (h *RuntimeHandlers) fetchRuleInstances() ([]*runtime.RuleInstance, error) {
	if h.executorURL != "" {
		url := h.executorURL + "/runtime/instances"
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}

		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				return nil, fmt.Errorf("executor returned status %d: %s", resp.StatusCode, string(body))
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, err
			}

			var result struct {
				Instances []*runtime.RuleInstance `json:"instances"`
				Count     int                     `json:"count"`
			}
			if err := json.Unmarshal(body, &result); err != nil {
				return nil, err
			}
			return result.Instances, nil
		}
	}

	if h.runtimeMgr != nil {
		return h.runtimeMgr.ListRuleInstances(), nil
	}

	return nil, fmt.Errorf("runtime manager not available and executor not reachable")
}

func (h *RuntimeHandlers) GetRuleInstanceHandler(c *gin.Context) {
	if !h.useLocalRuntime() {
		h.proxyToExecutor(c)
		return
	}

	instance, err := h.runtimeMgr.GetRuleInstance(c.Param("instance_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, h.instanceResponse(instance))
}

func (h *RuntimeHandlers) GetRuleInstanceByRuleIDHandler(c *gin.Context) {
	if !h.useLocalRuntime() {
		h.proxyToExecutor(c)
		return
	}

	instance, err := h.runtimeMgr.GetRuleInstanceByRuleID(c.Param("rule_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, h.instanceResponse(instance))
}

func (h *RuntimeHandlers) GetNodeInstanceHandler(c *gin.Context) {
	if !h.useLocalRuntime() {
		h.proxyToExecutor(c)
		return
	}

	instance, err := h.runtimeMgr.GetRuleInstance(c.Param("instance_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	nodeID := c.Param("node_id")
	node, ok := instance.Nodes[nodeID]
	if !ok {
		for _, candidate := range instance.Nodes {
			if candidate.NodeID == nodeID || candidate.ID == nodeID {
				node = candidate
				ok = true
				break
			}
		}
	}
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "node instance not found"})
		return
	}

	c.JSON(http.StatusOK, node)
}

func (h *RuntimeHandlers) GetEdgeInstanceHandler(c *gin.Context) {
	if !h.useLocalRuntime() {
		h.proxyToExecutor(c)
		return
	}

	instance, err := h.runtimeMgr.GetRuleInstance(c.Param("instance_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	edge, ok := instance.Edges[c.Param("edge_id")]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "edge instance not found"})
		return
	}

	c.JSON(http.StatusOK, edge)
}

func (h *RuntimeHandlers) ListNodeInstancesHandler(c *gin.Context) {
	if !h.useLocalRuntime() {
		h.proxyToExecutor(c)
		return
	}

	instance, err := h.runtimeMgr.GetRuleInstance(c.Param("instance_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	nodes := make([]*runtime.NodeInstance, 0, len(instance.Nodes))
	for _, node := range instance.Nodes {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].NodeID < nodes[j].NodeID
	})

	c.JSON(http.StatusOK, gin.H{
		"nodes":  nodes,
		"count":  len(nodes),
		"ruleId": instance.RuleID,
	})
}

func (h *RuntimeHandlers) ListEdgeInstancesHandler(c *gin.Context) {
	if !h.useLocalRuntime() {
		h.proxyToExecutor(c)
		return
	}

	instance, err := h.runtimeMgr.GetRuleInstance(c.Param("instance_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	edges := make([]*runtime.EdgeInstance, 0, len(instance.Edges))
	for _, edge := range instance.Edges {
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool {
		return edges[i].EdgeID < edges[j].EdgeID
	})

	c.JSON(http.StatusOK, gin.H{
		"edges":  edges,
		"count":  len(edges),
		"ruleId": instance.RuleID,
	})
}

func (h *RuntimeHandlers) GetTraceEventsHandler(c *gin.Context) {
	if !h.useLocalRuntime() {
		h.proxyToExecutor(c)
		return
	}

	instance, err := h.runtimeMgr.GetRuleInstance(c.Param("instance_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	limit := 100
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	if !instance.DebugMode {
		c.JSON(http.StatusOK, gin.H{
			"debug_enabled": false,
			"events":        []*runtime.TraceEvent{},
			"count":         0,
			"message":       "debug mode disabled",
		})
		return
	}

	events, err := h.runtimeMgr.GetTraceEvents(c.Param("instance_id"), limit, c.Query("node_id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"debug_enabled": true,
		"events":        events,
		"count":         len(events),
	})
}

func (h *RuntimeHandlers) GetCustomMetricsHandler(c *gin.Context) {
	if !h.useLocalRuntime() {
		h.proxyToExecutor(c)
		return
	}

	limit := 100
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	metrics, err := h.runtimeMgr.GetCustomMetrics(
		c.Param("instance_id"),
		c.Query("rule_id"),
		c.Query("node_id"),
		c.Query("name"),
		limit,
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"metrics": metrics,
		"count":   len(metrics),
	})
}

func (h *RuntimeHandlers) GetDLQEventsHandler(c *gin.Context) {
	if !h.useLocalRuntime() {
		h.proxyToExecutor(c)
		return
	}

	limit := 100
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	since, err := parseRuntimeRFC3339(c.Query("since"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid since, must be RFC3339"})
		return
	}
	until, err := parseRuntimeRFC3339(c.Query("until"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid until, must be RFC3339"})
		return
	}

	events, err := listRuntimeDLQMessages(c.Query("rule_id"), c.Query("node_id"), limit, since, until)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"events": events,
		"count":  len(events),
	})
}

func (h *RuntimeHandlers) ReplayDLQEventsHandler(c *gin.Context) {
	if !h.useLocalRuntime() {
		h.proxyToExecutor(c)
		return
	}

	var req struct {
		ID     string `json:"id"`
		RuleID string `json:"rule_id"`
		NodeID string `json:"node_id"`
		Limit  int    `json:"limit"`
		Since  string `json:"since"`
		Until  string `json:"until"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	since, err := parseRuntimeRFC3339(req.Since)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid since, must be RFC3339"})
		return
	}
	until, err := parseRuntimeRFC3339(req.Until)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid until, must be RFC3339"})
		return
	}

	limit := req.Limit
	if limit <= 0 {
		limit = 100
	}

	var candidates []*dlq.Message
	if req.ID != "" {
		msg, err := findRuntimeDLQMessageByID(req.ID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		candidates = []*dlq.Message{msg}
	} else {
		candidates, err = listRuntimeDLQMessages(req.RuleID, req.NodeID, limit, since, until)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	replayID := uuid.New().String()
	type replayFailure struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	}
	failures := make([]replayFailure, 0)
	succeeded := 0

	for _, msg := range candidates {
		replayMsg := cloneRuntimeDLQMessage(msg)
		if replayMsg.Event.Metadata == nil {
			replayMsg.Event.Metadata = make(map[string]string)
		}
		replayMsg.Event.Metadata["replay"] = "true"
		replayMsg.Event.Metadata["replay_id"] = replayID
		replayMsg.Event.Metadata["replayed_at"] = time.Now().Format(time.RFC3339)

		if err := executorsvc.ReplayDLQMessageLocal(replayMsg); err != nil {
			failures = append(failures, replayFailure{ID: replayMsg.ID, Error: err.Error()})
			continue
		}
		succeeded++
	}

	c.JSON(http.StatusOK, gin.H{
		"replay_id": replayID,
		"attempted": len(candidates),
		"succeeded": succeeded,
		"failed":    len(failures),
		"errors":    failures,
	})
}

func (h *RuntimeHandlers) instanceResponse(instance *runtime.RuleInstance) map[string]interface{} {
	instanceMap := map[string]interface{}{
		"id":         instance.ID,
		"rule_id":    instance.RuleID,
		"version":    instance.Version,
		"debug_mode": instance.DebugMode,
		"status":     instance.Status,
		"created_at": instance.CreatedAt,
		"started_at": instance.StartedAt,
		"nodes":      instance.Nodes,
		"edges":      instance.Edges,
		"metrics":    instance.Metrics,
		"name":       h.lookupRuleName(instance.RuleID),
	}
	if instance.StoppedAt != nil {
		instanceMap["stopped_at"] = instance.StoppedAt
	}
	return instanceMap
}

func (h *RuntimeHandlers) lookupRuleName(ruleID string) string {
	if h.manager != nil {
		if rule, err := h.manager.db.GetRule(ruleID); err == nil {
			return rule.Name
		}
	}
	return ruleID
}

func parseRuntimeRFC3339(raw string) (*time.Time, error) {
	if raw == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func listRuntimeDLQMessages(ruleID, nodeID string, limit int, since, until *time.Time) ([]*dlq.Message, error) {
	js := bus.GetJetStream()
	if js == nil {
		return []*dlq.Message{}, nil
	}

	info, err := js.StreamInfo("RULE_DLQ")
	if err != nil || info == nil {
		return []*dlq.Message{}, nil
	}

	messages := make([]*dlq.Message, 0)
	for seq := info.State.FirstSeq; seq <= info.State.LastSeq; seq++ {
		raw, err := js.GetMsg("RULE_DLQ", seq)
		if err != nil || raw == nil {
			continue
		}

		var msg dlq.Message
		if err := json.Unmarshal(raw.Data, &msg); err != nil {
			continue
		}
		if ruleID != "" && msg.RuleID != ruleID {
			continue
		}
		if nodeID != "" && msg.NodeID != nodeID {
			continue
		}
		if since != nil && msg.FailedAt.Before(*since) {
			continue
		}
		if until != nil && msg.FailedAt.After(*until) {
			continue
		}
		messages = append(messages, &msg)
	}

	sort.Slice(messages, func(i, j int) bool {
		return messages[i].FailedAt.After(messages[j].FailedAt)
	})

	if limit > 0 && len(messages) > limit {
		messages = messages[:limit]
	}
	return messages, nil
}

func findRuntimeDLQMessageByID(id string) (*dlq.Message, error) {
	items, err := listRuntimeDLQMessages("", "", 0, nil, nil)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, nil
		}
	}
	return nil, fmt.Errorf("dlq event not found: %s", id)
}

func cloneRuntimeDLQMessage(src *dlq.Message) *dlq.Message {
	if src == nil {
		return nil
	}
	dst := *src
	evt := src.Event
	if src.Event.Metadata != nil {
		evt.Metadata = make(map[string]string, len(src.Event.Metadata))
		for k, v := range src.Event.Metadata {
			evt.Metadata[k] = v
		}
	}
	dst.Event = evt
	return &dst
}
