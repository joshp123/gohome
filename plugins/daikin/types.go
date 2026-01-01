package daikin

// Device represents a Daikin unit with resolved metadata.
type Device struct {
	ID               string
	Name             string
	Model            string
	ClimateControlID string
	CloudConnected   bool
}
