package oauth

const (
	FlowAuthCode = "auth_code"
	FlowDevice   = "device"
)

// Declaration defines the OAuth contract a plugin must provide.
type Declaration struct {
	Provider       string
	Flow           string
	AuthorizeURL   string
	TokenURL       string
	DeviceAuthURL  string
	DeviceTokenURL string
	Scope          string
	StatePath      string
}
