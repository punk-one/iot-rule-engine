package version

import "fmt"

var Version = "dev"
var Commit = "unknown"
var BuiltAt = "unknown"

func String() string { return fmt.Sprintf("iot-rule-engine %s (%s, %s)", Version, Commit, BuiltAt) }
