package hikvision

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	hikvisionv1 "github.com/joshp123/gohome/proto/gen/plugins/hikvision/v1"
)

// ISAPI XML, decoded by local element name; the camera's namespace is ignored.

type deviceInfoXML struct {
	DeviceName          string `xml:"deviceName"`
	Model               string `xml:"model"`
	SerialNumber        string `xml:"serialNumber"`
	MacAddress          string `xml:"macAddress"`
	FirmwareVersion     string `xml:"firmwareVersion"`
	FirmwareBuild       string `xml:"firmwareReleasedDate"`
	EncoderVersion      string `xml:"encoderVersion"`
	HardwareVersion     string `xml:"hardwareVersion"`
	FirmwareVersionInfo string `xml:"firmwareVersionInfo"`
}

type motionDetectionXML struct {
	Enabled     bool   `xml:"enabled"`
	Sensitivity int32  `xml:"MotionDetectionLayout>sensitivityLevel"`
	TargetType  string `xml:"MotionDetectionLayout>targetType"`
}

// targetXML is a detectionTarget element: its value, and in capability
// responses the options in its opt attribute.
type targetXML struct {
	Value   string `xml:",chardata"`
	Options string `xml:"opt,attr"`
}

type smartRuleXML struct {
	ID          uint32    `xml:"id"`
	Enabled     bool      `xml:"enabled"`
	Sensitivity int32     `xml:"sensitivityLevel"`
	Target      targetXML `xml:"detectionTarget"`
}

// smartDetectionXML covers LineDetection (rules are LineItem) and
// FieldDetection (rules are FieldDetectionRegion), and their capabilities.
type smartDetectionXML struct {
	Enabled bool           `xml:"enabled"`
	Lines   []smartRuleXML `xml:"LineItemList>LineItem"`
	Regions []smartRuleXML `xml:"FieldDetectionRegionList>FieldDetectionRegion"`
}

func (s smartDetectionXML) rules() []smartRuleXML {
	return append(append([]smartRuleXML{}, s.Lines...), s.Regions...)
}

type eventNotificationXML struct {
	ChannelID       uint32 `xml:"channelID"`
	DateTime        string `xml:"dateTime"`
	EventType       string `xml:"eventType"`
	EventState      string `xml:"eventState"`
	DetectionTarget string `xml:"detectionTarget"`
	Regions         []struct {
		RegionID        uint32 `xml:"regionID"`
		DetectionTarget string `xml:"detectionTarget"`
	} `xml:"DetectionRegionList>DetectionRegionEntry"`
}

func decodeXML(source string, body []byte, dest any) error {
	if err := xml.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("decode %s: %w", source, err)
	}
	return nil
}

// supportedCapabilities returns X for every <isSupportX>true</isSupportX>
// inside the named section (EventCap or SmartCap), wherever the section sits.
func supportedCapabilities(body []byte, section string) ([]string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(body))
	depth, sectionDepth := 0, 0
	var supported []string
	var element string
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			element = t.Name.Local
			if element == section && sectionDepth == 0 {
				sectionDepth = depth
			}
		case xml.EndElement:
			if depth == sectionDepth {
				sectionDepth = 0
			}
			depth--
			element = ""
		case xml.CharData:
			if sectionDepth != 0 && strings.HasPrefix(element, "isSupport") && strings.TrimSpace(string(t)) == "true" {
				supported = append(supported, strings.TrimPrefix(element, "isSupport"))
			}
		}
	}
	return supported, nil
}

func mergeSorted(lists ...[]string) []string {
	seen := map[string]bool{}
	var merged []string
	for _, list := range lists {
		for _, item := range list {
			if !seen[item] {
				seen[item] = true
				merged = append(merged, item)
			}
		}
	}
	sort.Strings(merged)
	return merged
}

// parseTarget maps the camera's detection-target word(s) to the fixed vocabulary.
func parseTarget(raw string) hikvisionv1.DetectionTarget {
	words := map[string]bool{}
	for _, word := range strings.Split(raw, ",") {
		if word = strings.ToLower(strings.TrimSpace(word)); word != "" {
			words[word] = true
		}
	}
	switch {
	case len(words) == 0:
		return hikvisionv1.DetectionTarget_DETECTION_TARGET_UNSPECIFIED
	case words["all"] || (len(words) == 2 && words["human"] && words["vehicle"]):
		return hikvisionv1.DetectionTarget_DETECTION_TARGET_ALL
	case len(words) == 1 && words["human"]:
		return hikvisionv1.DetectionTarget_DETECTION_TARGET_HUMAN
	case len(words) == 1 && words["vehicle"]:
		return hikvisionv1.DetectionTarget_DETECTION_TARGET_VEHICLE
	default:
		return hikvisionv1.DetectionTarget_DETECTION_TARGET_OTHER
	}
}

// parseTargetOptions reads a list such as "human,vehicle" or an opt attribute such as "all,human,vehicle".
func parseTargetOptions(raw string) []hikvisionv1.DetectionTarget {
	var targets []hikvisionv1.DetectionTarget
	for _, word := range strings.Split(raw, ",") {
		if strings.TrimSpace(word) != "" {
			targets = append(targets, parseTarget(word))
		}
	}
	return targets
}

var eventKinds = map[string]hikvisionv1.EventKind{
	"vmd":                  hikvisionv1.EventKind_EVENT_KIND_MOTION,
	"linedetection":        hikvisionv1.EventKind_EVENT_KIND_LINE_CROSSING,
	"fielddetection":       hikvisionv1.EventKind_EVENT_KIND_INTRUSION,
	"regionentrance":       hikvisionv1.EventKind_EVENT_KIND_REGION_ENTRANCE,
	"regionexiting":        hikvisionv1.EventKind_EVENT_KIND_REGION_EXITING,
	"shelteralarm":         hikvisionv1.EventKind_EVENT_KIND_TAMPER,
	"tamperdetection":      hikvisionv1.EventKind_EVENT_KIND_TAMPER,
	"scenechangedetection": hikvisionv1.EventKind_EVENT_KIND_SCENE_CHANGE,
	"videoloss":            hikvisionv1.EventKind_EVENT_KIND_VIDEO_LOSS,
}

func parseEventKind(raw string) hikvisionv1.EventKind {
	if kind, ok := eventKinds[strings.ToLower(raw)]; ok {
		return kind
	}
	return hikvisionv1.EventKind_EVENT_KIND_OTHER
}

func parseEventState(raw string) hikvisionv1.EventState {
	switch strings.ToLower(raw) {
	case "active":
		return hikvisionv1.EventState_EVENT_STATE_ACTIVE
	case "inactive":
		return hikvisionv1.EventState_EVENT_STATE_INACTIVE
	default:
		return hikvisionv1.EventState_EVENT_STATE_UNSPECIFIED
	}
}

// cameraEvent is one decoded alert-stream notification.
type cameraEvent struct {
	Kind        hikvisionv1.EventKind
	State       hikvisionv1.EventState
	Target      hikvisionv1.DetectionTarget
	CameraTime  time.Time // zero when the camera's timestamp does not parse
	ReceivedAt  time.Time
	ChannelID   uint32
	RuleID      uint32
	EventType   string
	CameraWords string // the camera's detection-target word(s)
}

// isKeepAlive: with nothing to report, the camera posts "videoloss inactive"
// every ten seconds. It does not support video-loss detection at all.
func (e cameraEvent) isKeepAlive() bool {
	return e.Kind == hikvisionv1.EventKind_EVENT_KIND_VIDEO_LOSS && e.State == hikvisionv1.EventState_EVENT_STATE_INACTIVE
}

func parseEventNotification(body []byte, receivedAt time.Time) (cameraEvent, error) {
	var raw eventNotificationXML
	if err := decodeXML("EventNotificationAlert", body, &raw); err != nil {
		return cameraEvent{}, err
	}
	if raw.EventType == "" {
		return cameraEvent{}, fmt.Errorf("EventNotificationAlert has no eventType")
	}
	event := cameraEvent{
		Kind:        parseEventKind(raw.EventType),
		State:       parseEventState(raw.EventState),
		ReceivedAt:  receivedAt,
		ChannelID:   raw.ChannelID,
		EventType:   raw.EventType,
		CameraWords: raw.DetectionTarget,
	}
	if len(raw.Regions) > 0 {
		event.RuleID = raw.Regions[0].RegionID
		if event.CameraWords == "" {
			event.CameraWords = raw.Regions[0].DetectionTarget
		}
	}
	event.Target = parseTarget(event.CameraWords)
	if t, err := time.Parse(time.RFC3339, raw.DateTime); err == nil {
		event.CameraTime = t
	}
	return event, nil
}
