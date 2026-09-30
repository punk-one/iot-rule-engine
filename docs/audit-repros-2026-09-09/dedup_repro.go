//go:build ignore

// Isolated full-queue diagnostic; no worker, NATS, PLC, or broker is started.
package main

import (
	"fmt"
	"github.com/punk-one/iot-rule-engine/internal/core/dispatcher"
	sdk "github.com/punk-one/iot-rule-plugin-sdk"
	"time"
)

func main() {
	d := dispatcher.NewDispatcherWithSecurityConfig("audit", time.Second, false, "best_effort", nil, 1, 100, 100, 1000, 256, "reject")
	d.RegisterRule("audit", map[string][]string{"source": {"sink"}})
	a, b := sdk.NewEvent(nil, nil), sdk.NewEvent(nil, nil)
	fill := d.OnPluginEmit("audit", "source", &a)
	first := d.OnPluginEmit("audit", "source", &b)
	retry := d.OnPluginEmit("audit", "source", &b)
	fmt.Printf("fill_ok=%v first_attempt_rejected=%v retry_falsely_accepted=%v\n", fill == nil, first != nil, retry == nil)
}
