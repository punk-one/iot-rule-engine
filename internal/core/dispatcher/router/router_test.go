package router

import (
	"testing"

	"github.com/punk-one/iot-rule-plugin-sdk"
)

func TestRouter_Resolve(t *testing.T) {
	// Setup Routes
	// Node A -> (Label: "ok") -> Node B
	// Node A -> (Label: "err") -> Node C

	router := NewRouter()
	router.RegisterRouter("A", map[string]string{
		"ok":  "B",
		"err": "C",
	})

	tests := []struct {
		name      string
		fromNode  string
		event     string // "EmitTo" label
		wantTo    string
		wantFound bool
	}{
		{"Match OK", "A", "ok", "B", true},
		{"Match Err", "A", "err", "C", true},
		{"No Match Label", "A", "other", "", false},
		{"No Match Node", "X", "ok", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Construct Event
			meta := map[string]string{}
			if tt.event != "" {
				meta["route_label"] = tt.event
			}
			evt := sdk.NewEvent(nil, meta)
			evt.Source = tt.fromNode

			gotTos := router.Resolve(evt)

			if !tt.wantFound {
				if len(gotTos) > 0 {
					t.Errorf("Resolve() found %v, want none", gotTos)
				}
				return
			}

			if len(gotTos) == 0 {
				t.Errorf("Resolve() found none, want %s", tt.wantTo)
				return
			}

			if gotTos[0] != tt.wantTo {
				t.Errorf("Resolve() gotTo = %v, want %v", gotTos[0], tt.wantTo)
			}
		})
	}
}
