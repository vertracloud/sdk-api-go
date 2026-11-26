package vertracloud

// RegionID is the region where an application or database runs. It is chosen on creation and never changes.
type RegionID string

const (
	RegionNewYork  RegionID = "ny"
	RegionSaoPaulo RegionID = "sp"
)
