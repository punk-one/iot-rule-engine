package engine

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestBuildTraceMessageUsesCompactPayloadSummary(t *testing.T) {
	payload := map[string]interface{}{
		"data": map[string]interface{}{
			"air_lo_alm":   false,
			"gas_flow_rt":  14.80942,
			"gas_hr_sum":   8000.3564,
			"hi_press_alm": false,
			"lo_press_alm": false,
			"pwr_hr_sum":   3112.4,
			"stf_atm_tmp":  779,
			"stf_met_tmp":  745,
			"tap_ratio_tm": 0,
			"tap_temp":     733,
		},
		"device_code": "ZH_JZL_0001",
		"time":        int64(1776150552946),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	message := buildTraceMessage(payload, "success")
	if !strings.Contains(message, "status=success") {
		t.Fatalf("expected status summary, got %q", message)
	}
	if !strings.Contains(message, "payload_bytes=") {
		t.Fatalf("expected payload size summary, got %q", message)
	}
	if !strings.Contains(message, "payload_bytes="+strconv.Itoa(len(payloadBytes))) {
		t.Fatalf("expected payload size in summary, got %q", message)
	}
	if strings.Contains(message, `"tap_temp":733`) {
		t.Fatalf("expected compact trace summary without full payload duplication, got %q", message)
	}
}
