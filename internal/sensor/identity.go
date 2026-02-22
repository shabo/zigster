package sensor

import "strings"

// chipIdentityMap maps chip name prefixes to friendly component names.
var chipIdentityMap = []struct {
	prefix string
	name   string
}{
	{"coretemp", "CPU"},
	{"k10temp", "CPU"},
	{"zenpower", "CPU"},
	{"amdgpu", "GPU (AMD)"},
	{"radeon", "GPU (AMD)"},
	{"nouveau", "GPU (NVIDIA)"},
	{"nvidia-gpu", "GPU (NVIDIA)"},
	{"nvidia", "GPU (NVIDIA)"},
	{"intel_gpu", "GPU (Intel)"},
	{"i915", "GPU (Intel)"},
	{"nvme", "NVMe SSD"},
	{"drivetemp", "HDD/SSD"},
	{"smart-", "HDD/SSD"},
	{"iwlwifi", "WiFi"},
	{"ath", "WiFi"},
	{"mt7", "WiFi"},
	{"rtw", "WiFi"},
	{"pch", "PCH (Chipset)"},
	{"acpi", "ACPI Thermal"},
	{"it87", "Motherboard"},
	{"nct", "Motherboard"},
	{"w83", "Motherboard"},
	{"f71", "Motherboard"},
	{"asus", "Motherboard"},
	{"thinkpad", "Laptop EC"},
	{"dell", "Laptop EC"},
	{"hp", "Laptop EC"},
	{"bat", "Battery"},
}

// FriendlyName returns a human-readable component name for a chip ID.
func FriendlyName(chip string) string {
	lower := strings.ToLower(chip)
	for _, entry := range chipIdentityMap {
		if strings.HasPrefix(lower, entry.prefix) {
			return entry.name
		}
	}
	return "Sensor"
}
