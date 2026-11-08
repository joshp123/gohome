package sonos

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const requestTimeout = 5 * time.Second

// readAction is one UPnP SOAP read. The client can send only the actions declared below:
// nothing in this package plays, pauses, seeks or changes volume.
type readAction struct {
	controlPath string
	service     string
	name        string
	arguments   string
}

var (
	getZoneGroupState = readAction{"/ZoneGroupTopology/Control", "ZoneGroupTopology", "GetZoneGroupState", ""}
	getTransportInfo  = readAction{"/MediaRenderer/AVTransport/Control", "AVTransport", "GetTransportInfo", "<InstanceID>0</InstanceID>"}
	getPositionInfo   = readAction{"/MediaRenderer/AVTransport/Control", "AVTransport", "GetPositionInfo", "<InstanceID>0</InstanceID>"}
	getVolume         = readAction{"/MediaRenderer/RenderingControl/Control", "RenderingControl", "GetVolume", "<InstanceID>0</InstanceID><Channel>Master</Channel>"}
	getMute           = readAction{"/MediaRenderer/RenderingControl/Control", "RenderingControl", "GetMute", "<InstanceID>0</InstanceID><Channel>Master</Channel>"}
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(cfg Config) (*Client, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("sonos base_url is required")
	}
	return &Client{baseURL: cfg.BaseURL, httpClient: &http.Client{Timeout: requestTimeout}}, nil
}

// Zones reads the system topology from the configured player.
func (c *Client) Zones(ctx context.Context) ([]Zone, error) {
	values, err := c.read(ctx, c.baseURL, getZoneGroupState)
	if err != nil {
		return nil, err
	}
	return parseZoneGroupState(values["ZoneGroupState"])
}

// Status reads transport state, volume and mute from a zone's coordinator.
func (c *Client) Status(ctx context.Context, zone Zone) (ZoneStatus, error) {
	baseURL := "http://" + zone.CoordinatorHost
	transport, err := c.read(ctx, baseURL, getTransportInfo)
	if err != nil {
		return ZoneStatus{}, err
	}
	volume, err := c.read(ctx, baseURL, getVolume)
	if err != nil {
		return ZoneStatus{}, err
	}
	mute, err := c.read(ctx, baseURL, getMute)
	if err != nil {
		return ZoneStatus{}, err
	}
	volumeLevel, err := strconv.ParseUint(volume["CurrentVolume"], 10, 8)
	if err != nil {
		return ZoneStatus{}, fmt.Errorf("zone %q volume %q: %w", zone.Name, volume["CurrentVolume"], err)
	}
	return ZoneStatus{
		TransportState: transport["CurrentTransportState"],
		Volume:         uint32(volumeLevel),
		Muted:          mute["CurrentMute"] == "1",
	}, nil
}

// Track reads what a zone's coordinator has loaded. It returns nil when nothing is.
func (c *Client) Track(ctx context.Context, zone Zone) (*Track, error) {
	values, err := c.read(ctx, "http://"+zone.CoordinatorHost, getPositionInfo)
	if err != nil {
		return nil, err
	}
	return parsePositionInfo(values)
}

// read sends one SOAP read and returns the text of each element in the response.
func (c *Client) read(ctx context.Context, baseURL string, action readAction) (map[string]string, error) {
	serviceType := "urn:schemas-upnp-org:service:" + action.service + ":1"
	envelope := `<?xml version="1.0" encoding="utf-8"?>` +
		`<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>` +
		`<u:` + action.name + ` xmlns:u="` + serviceType + `">` + action.arguments + `</u:` + action.name + `>` +
		`</s:Body></s:Envelope>`
	endpoint := baseURL + action.controlPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(envelope))
	if err != nil {
		return nil, fmt.Errorf("build %s request: %w", action.name, err)
	}
	req.Header.Set("Content-Type", `text/xml; charset="utf-8"`)
	req.Header.Set("SOAPACTION", `"`+serviceType+"#"+action.name+`"`)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", action.name, endpoint, err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("%s %s: read body: %w", action.name, endpoint, err)
	}
	values, err := soapValues(payload)
	if err != nil {
		return nil, fmt.Errorf("%s %s: %w", action.name, endpoint, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s %s: HTTP %d, UPnP error %s", action.name, endpoint, resp.StatusCode, values["errorCode"])
	}
	return values, nil
}

// soapValues maps each leaf element's local name to its text. SOAP responses here are flat,
// and the decoder unescapes embedded XML (ZoneGroupState, TrackMetaData) for us.
func soapValues(payload []byte) (map[string]string, error) {
	values := make(map[string]string)
	decoder := xml.NewDecoder(strings.NewReader(string(payload)))
	var current string
	var text strings.Builder
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return values, nil
		}
		if err != nil {
			return nil, fmt.Errorf("decode SOAP response: %w", err)
		}
		switch t := token.(type) {
		case xml.StartElement:
			current = t.Name.Local
			text.Reset()
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			if t.Name.Local == current {
				values[current] = text.String()
			}
			current = ""
		}
	}
}
