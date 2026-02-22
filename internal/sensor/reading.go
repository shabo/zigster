// Package sensor provides dynamic hardware temperature sensor discovery.
// It combines lm-sensors (JSON + text fallback), nvidia-smi, and
// smartctl/drivetemp to produce a unified view of all thermal sensors.
package sensor

// Reading represents a single temperature reading from a sensor.
type Reading struct {
	Chip    string  // e.g. "coretemp-isa-0000"
	Adapter string  // e.g. "ISA adapter"
	Label   string  // e.g. "Core 0"
	Temp    float64 // current temperature in Celsius
	High    float64 // high threshold (0 if not available)
	Crit    float64 // critical threshold (0 if not available)
	HasHigh bool
	HasCrit bool
}

// Key returns a unique identifier for this sensor.
func (r Reading) Key() string {
	return r.Chip + "/" + r.Label
}
