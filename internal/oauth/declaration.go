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
	// ClientID and ClientSecret are set only for a provider that publishes its client credentials.
	// Such a provider needs no bootstrap file.
	ClientID     string
	ClientSecret string
}
