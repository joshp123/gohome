package hikvision

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// A past moment's picture comes from the camera's own recording on its
// memory card: an RTSP playback that starts at that second, of which ffmpeg
// decodes the first picture. Playback only reads; the recording is unchanged.
const (
	recordedMainTrack = "101"
	rtspPort          = "554"
	frameTimeout      = 20 * time.Second
	frameCacheSize    = 8
	frameCacheFor     = 5 * time.Minute
)

var (
	errNoRecording = errors.New("the camera has no recording at that time")
	errNoFFmpeg    = errors.New("ffmpeg is not installed, so recorded pictures cannot be decoded")
)

type recordingSearchXML struct {
	XMLName    xml.Name `xml:"CMSearchDescription"`
	SearchID   string   `xml:"searchID"`
	TrackIDs   []string `xml:"trackIDList>trackID"`
	Start      string   `xml:"timeSpanList>timeSpan>startTime"`
	End        string   `xml:"timeSpanList>timeSpan>endTime"`
	MaxResults int      `xml:"maxResults"`
	Position   int      `xml:"searchResultPostion"` // sic
	Metadata   string   `xml:"metadataList>metadataDescriptor"`
}

type recordingSearchResultXML struct {
	Matches []struct {
		Start string `xml:"timeSpan>startTime"`
		End   string `xml:"timeSpan>endTime"`
	} `xml:"matchList>searchMatchItem"`
}

// recordingCovers says whether the main-stream recording covers the second at.
func (c *Client) recordingCovers(ctx context.Context, at time.Time) (bool, error) {
	search := recordingSearchXML{
		SearchID:   newSearchID(),
		TrackIDs:   []string{recordedMainTrack},
		Start:      at.UTC().Format(isapiTimeLayout),
		End:        at.Add(time.Second).UTC().Format(isapiTimeLayout),
		MaxResults: 1,
		Metadata:   "//recordType.meta.std-cgi.com",
	}
	var result recordingSearchResultXML
	if err := c.search(ctx, pathRecordingSearch, search, &result); err != nil {
		return false, err
	}
	for _, match := range result.Matches {
		start, startErr := time.Parse(isapiTimeLayout, match.Start)
		end, endErr := time.Parse(isapiTimeLayout, match.End)
		if startErr == nil && endErr == nil && !at.Before(start) && at.Before(end) {
			return true, nil
		}
	}
	return false, nil
}

// strictURLEscape percent-encodes everything but unreserved characters, so a
// password cannot break out of the URL or the ffmpeg playlist quoting.
func strictURLEscape(s string) string {
	var b strings.Builder
	for _, c := range []byte(s) {
		if ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') || strings.IndexByte("-._~", c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// playbackPlaylist is an ffmpeg concat playlist naming the RTSP playback that
// starts at the second at. It holds the credentials, so it is written to a
// private file rather than passed on ffmpeg's command line.
func (c *Client) playbackPlaylist(at time.Time) (string, error) {
	base, err := url.Parse(c.baseURL)
	if err != nil || base.Hostname() == "" {
		return "", fmt.Errorf("camera base_url %q has no host", c.baseURL)
	}
	playback := fmt.Sprintf("rtsp://%s:%s@%s/Streaming/tracks/%s/?starttime=%s",
		strictURLEscape(c.username), strictURLEscape(c.password),
		net.JoinHostPort(base.Hostname(), rtspPort), recordedMainTrack, at.UTC().Format("20060102T150405Z"))
	return "ffconcat version 1.0\nfile '" + playback + "'\noption rtsp_transport tcp\n", nil
}

// recordedFrames reads pictures from the recording one at a time, and keeps
// the last few in memory briefly so a dashboard refresh does not replay them.
type recordedFrames struct {
	client *Client

	mu    sync.Mutex
	cache map[int64]cachedFrame
}

type cachedFrame struct {
	jpeg    []byte
	expires time.Time
}

func newRecordedFrames(client *Client) *recordedFrames {
	return &recordedFrames{client: client, cache: map[int64]cachedFrame{}}
}

// Frame returns the first picture the recording holds from the second at.
func (f *recordedFrames) Frame(ctx context.Context, at time.Time) ([]byte, time.Time, error) {
	at = at.UTC().Truncate(time.Second)
	f.mu.Lock()
	defer f.mu.Unlock()
	now := time.Now()
	for second, cached := range f.cache {
		if now.After(cached.expires) {
			delete(f.cache, second)
		}
	}
	if cached, ok := f.cache[at.Unix()]; ok {
		return cached.jpeg, at, nil
	}
	ctx, cancel := context.WithTimeout(ctx, frameTimeout)
	defer cancel()
	covered, err := f.client.recordingCovers(ctx, at)
	if err != nil {
		return nil, at, err
	}
	if !covered {
		return nil, at, errNoRecording
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, at, errNoFFmpeg
	}
	jpeg, err := f.client.decodeFrame(ctx, ffmpeg, at)
	if err != nil {
		return nil, at, err
	}
	if len(f.cache) >= frameCacheSize {
		for second := range f.cache {
			delete(f.cache, second)
			break
		}
	}
	f.cache[at.Unix()] = cachedFrame{jpeg: jpeg, expires: now.Add(frameCacheFor)}
	return jpeg, at, nil
}

func (c *Client) decodeFrame(ctx context.Context, ffmpeg string, at time.Time) ([]byte, error) {
	playlist, err := c.playbackPlaylist(at)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "gohome-hikvision-frame-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	playlistPath := filepath.Join(dir, "playback.ffconcat")
	if err := os.WriteFile(playlistPath, []byte(playlist), 0o600); err != nil {
		return nil, err
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-loglevel", "error",
		"-f", "concat", "-safe", "0", "-protocol_whitelist", "file,rtsp,rtp,tcp,udp",
		"-i", playlistPath, "-frames:v", "1", "-f", "image2pipe", "-c:v", "mjpeg", "-q:v", "3", "pipe:1")
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		// ffmpeg's message can quote the playlist line; keep the credentials out of it.
		message := strings.ReplaceAll(strings.TrimSpace(stderr.String()), strictURLEscape(c.password), "***")
		return nil, fmt.Errorf("decode recorded picture: %v: %s", err, message)
	}
	jpeg := stdout.Bytes()
	if len(jpeg) < 4 || jpeg[0] != 0xFF || jpeg[1] != 0xD8 {
		return nil, fmt.Errorf("decode recorded picture: ffmpeg returned no JPEG")
	}
	if len(jpeg) > maxSnapshotBytes {
		return nil, fmt.Errorf("decode recorded picture: larger than %d bytes", maxSnapshotBytes)
	}
	return jpeg, nil
}
