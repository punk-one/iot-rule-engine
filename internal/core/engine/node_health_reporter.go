package engine

import (
    "sync/atomic"
    "time"

    sdk "github.com/punk-one/iot-rule-plugin-sdk"
    "github.com/punk-one/iot-rule-engine/internal/core/runtime"
)

type nodeHealthReporter struct {
    runtimeMgr    *runtime.Manager
    instanceID    string
    runtimeNodeID string
    sequence      atomic.Uint64
}

type nopNodeHealthReporter struct{}

func (nopNodeHealthReporter) ApplyHealthUpdate(sdk.ReportHealthArgs) error                       { return nil }
func (nopNodeHealthReporter) Report(sdk.HealthReport) error                                      { return nil }
func (nopNodeHealthReporter) Healthy(string, map[string]string) error                            { return nil }
func (nopNodeHealthReporter) Degraded(string, string, map[string]string) error                   { return nil }
func (nopNodeHealthReporter) Recovering(string, string, map[string]string) error                 { return nil }
func (nopNodeHealthReporter) Unhealthy(string, string, map[string]string) error                  { return nil }
func (nopNodeHealthReporter) Heartbeat(string, map[string]string) error                          { return nil }

func newNodeHealthReporter(runtimeMgr *runtime.Manager, instanceID, runtimeNodeID string) *nodeHealthReporter {
    return &nodeHealthReporter{
        runtimeMgr:    runtimeMgr,
        instanceID:    instanceID,
        runtimeNodeID: runtimeNodeID,
    }
}

func (r *nodeHealthReporter) ApplyHealthUpdate(update sdk.ReportHealthArgs) error {
    if r == nil || r.runtimeMgr == nil || r.instanceID == "" || r.runtimeNodeID == "" {
        return nil
    }
    return r.runtimeMgr.ApplyNodeHealthUpdate(r.instanceID, r.runtimeNodeID, update)
}

func (r *nodeHealthReporter) Report(report sdk.HealthReport) error {
    if report.Status == "" {
        report.Status = sdk.HealthUnknown
    }
    if report.ObservedAt.IsZero() {
        report.ObservedAt = time.Now()
    }
    report.Sequence = r.sequence.Add(1)
    return r.ApplyHealthUpdate(sdk.ReportHealthArgs{
        Kind:               sdk.HealthKindState,
        Status:             report.Status,
        Code:               report.Code,
        Message:            report.Message,
        Details:            report.Details,
        ObservedAtUnixNano: report.ObservedAt.UnixNano(),
        Sequence:           report.Sequence,
    })
}

func (r *nodeHealthReporter) Healthy(msg string, details map[string]string) error {
    return r.Report(sdk.HealthReport{Status: sdk.HealthHealthy, Message: msg, Details: details})
}

func (r *nodeHealthReporter) Degraded(code, msg string, details map[string]string) error {
    return r.Report(sdk.HealthReport{Status: sdk.HealthDegraded, Code: code, Message: msg, Details: details})
}

func (r *nodeHealthReporter) Recovering(code, msg string, details map[string]string) error {
    return r.Report(sdk.HealthReport{Status: sdk.HealthRecovering, Code: code, Message: msg, Details: details})
}

func (r *nodeHealthReporter) Unhealthy(code, msg string, details map[string]string) error {
    return r.Report(sdk.HealthReport{Status: sdk.HealthUnhealthy, Code: code, Message: msg, Details: details})
}

func (r *nodeHealthReporter) Heartbeat(msg string, details map[string]string) error {
    observedAt := time.Now()
    return r.ApplyHealthUpdate(sdk.ReportHealthArgs{
        Kind:               sdk.HealthKindHeartbeat,
        Message:            msg,
        Details:            details,
        ObservedAtUnixNano: observedAt.UnixNano(),
        Sequence:           r.sequence.Add(1),
    })
}
