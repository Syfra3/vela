package ir

// CapabilityList describes the intentionally limited Phase 1 Common IR surface.
type CapabilityList struct {
	Supported []string
}

// Phase1Capabilities returns the approved Phase 1 Common IR capability boundary.
func Phase1Capabilities() CapabilityList {
	return CapabilityList{Supported: []string{
		"Common IR node taxonomy",
		"Common IR edge taxonomy",
		"fact metadata",
		"freshness",
		"confidence",
		"migration",
		"compatibility",
		"enrichment approval contract",
	}}
}

// Includes reports whether the capability list explicitly includes a capability.
func (c CapabilityList) Includes(capability string) bool {
	for _, supported := range c.Supported {
		if supported == capability {
			return true
		}
	}
	return false
}

// Claims reports whether the capability list explicitly claims a capability.
func (c CapabilityList) Claims(capability string) bool {
	return c.Includes(capability)
}
