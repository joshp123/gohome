package roborock

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

var baseURLs = []string{
	"https://usiot.roborock.com",
	"https://euiot.roborock.com",
	"https://cniot.roborock.com",
	"https://ruiot.roborock.com",
}

type IotLoginInfo struct {
	BaseURL     string
	CountryCode string
	Country     string
}

type Reference struct {
	R string `json:"r"`
	A string `json:"a"`
	M string `json:"m"`
	L string `json:"l"`
}

type RRiot struct {
	U string    `json:"u"`
	S string    `json:"s"`
	H string    `json:"h"`
	K string    `json:"k"`
	R Reference `json:"r"`
}

type UserData struct {
	UID         int64  `json:"uid"`
	TokenType   string `json:"tokentype"`
	Token       string `json:"token"`
	RRUID       string `json:"rruid"`
	Region      string `json:"region"`
	CountryCode string `json:"countrycode"`
	Country     string `json:"country"`
	Nickname    string `json:"nickname"`
	RRIOT       RRiot  `json:"rriot"`
}

type HomeData struct {
	ID              int64             `json:"id"`
	Name            string            `json:"name"`
	Products        []HomeDataProduct `json:"products"`
	Devices         []HomeDataDevice  `json:"devices"`
	ReceivedDevices []HomeDataDevice  `json:"receivedDevices"`
}

type HomeDataProduct struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Model    string `json:"model"`
	Category string `json:"category"`
	Code     string `json:"code"`
}

type HomeDataDevice struct {
	DUID         string         `json:"duid"`
	Name         string         `json:"name"`
	LocalKey     string         `json:"localKey"`
	ProductID    string         `json:"productId"`
	Firmware     string         `json:"fv"`
	Online       bool           `json:"online"`
	DeviceStatus map[string]any `json:"deviceStatus"`
}

type RoborockApiClient struct {
	username   string
	baseURL    string
	httpClient *http.Client
	deviceID   string
	info       *IotLoginInfo
}

func NewRoborockApiClient(username, baseURL string) *RoborockApiClient {
	return &RoborockApiClient{
		username: username,
		baseURL:  baseURL,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		deviceID: randomDeviceID(),
	}
}

func randomDeviceID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (c *RoborockApiClient) getLoginInfo(ctx context.Context) (*IotLoginInfo, error) {
	if c.info != nil {
		return c.info, nil
	}
	urls := baseURLs
	if c.baseURL != "" {
		urls = []string{c.baseURL}
	}
	for _, base := range urls {
		resp, err := c.doRequest(ctx, "POST", base+"/api/v1/getUrlByEmail", map[string]string{
			"email":           c.username,
			"needtwostepauth": "false",
		}, nil, nil)
		if err != nil {
			continue
		}
		code, _ := resp["code"].(float64)
		if int(code) != 200 {
			return nil, fmt.Errorf("getUrlByEmail failed: %v", resp["msg"])
		}
		data, _ := resp["data"].(map[string]any)
		if data == nil {
			continue
		}
		country, _ := data["country"].(string)
		countryCode := fmt.Sprintf("%v", data["countrycode"])
		urlStr, _ := data["url"].(string)
		if urlStr == "" {
			continue
		}
		c.info = &IotLoginInfo{BaseURL: urlStr, Country: country, CountryCode: countryCode}
		return c.info, nil
	}
	return nil, errors.New("no response from any base url")
}

func (c *RoborockApiClient) baseURLOrLogin(ctx context.Context) (string, error) {
	if c.baseURL != "" {
		return c.baseURL, nil
	}
	info, err := c.getLoginInfo(ctx)
	if err != nil {
		return "", err
	}
	return info.BaseURL, nil
}

func (c *RoborockApiClient) BaseURL(ctx context.Context) (string, error) {
	return c.baseURLOrLogin(ctx)
}

func (c *RoborockApiClient) RequestCodeV4(ctx context.Context) error {
	base, err := c.baseURLOrLogin(ctx)
	if err != nil {
		return err
	}
	headers := map[string]string{
		"header_clientid":   c.headerClientID(),
		"Content-Type":      "application/x-www-form-urlencoded",
		"header_clientlang": "en",
	}
	resp, err := c.doRequest(ctx, "POST", base+"/api/v4/email/code/send", nil, headers, map[string]string{
		"email":    c.username,
		"type":     "login",
		"platform": "",
	})
	if err != nil {
		return err
	}
	code := int(asFloat(resp["code"]))
	if code != 200 {
		return fmt.Errorf("request code failed: %v", resp["msg"])
	}
	return nil
}

func (c *RoborockApiClient) signKeyV3(ctx context.Context, s string) (string, error) {
	base, err := c.baseURLOrLogin(ctx)
	if err != nil {
		return "", err
	}
	headers := map[string]string{"header_clientid": c.headerClientID()}
	resp, err := c.doRequest(ctx, "POST", base+"/api/v3/key/sign", map[string]string{"s": s}, headers, nil)
	if err != nil {
		return "", err
	}
	if int(asFloat(resp["code"])) != 200 {
		return "", fmt.Errorf("sign key failed: %v", resp["msg"])
	}
	data, _ := resp["data"].(map[string]any)
	key, _ := data["k"].(string)
	if key == "" {
		return "", errors.New("missing key in sign response")
	}
	return key, nil
}

func (c *RoborockApiClient) CodeLoginV4(ctx context.Context, code string) (map[string]any, error) {
	base, err := c.baseURLOrLogin(ctx)
	if err != nil {
		return nil, err
	}
	info, err := c.getLoginInfo(ctx)
	if err != nil {
		return nil, err
	}
	ks := randomAlphaNumeric(16)
	k, err := c.signKeyV3(ctx, ks)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{
		"header_clientid":    c.headerClientID(),
		"x-mercy-ks":         ks,
		"x-mercy-k":          k,
		"Content-Type":       "application/x-www-form-urlencoded",
		"header_clientlang":  "en",
		"header_appversion":  "4.54.02",
		"header_phonesystem": "iOS",
		"header_phonemodel":  "iPhone16,1",
	}
	resp, err := c.doRequest(ctx, "POST", base+"/api/v4/auth/email/login/code", nil, headers, map[string]string{
		"country":      info.Country,
		"countryCode":  info.CountryCode,
		"email":        c.username,
		"code":         code,
		"majorVersion": "14",
		"minorVersion": "0",
	})
	if err != nil {
		return nil, err
	}
	if int(asFloat(resp["code"])) != 200 {
		return nil, fmt.Errorf("login failed: %v", resp["msg"])
	}
	data, _ := resp["data"].(map[string]any)
	if data == nil {
		return nil, errors.New("missing user data")
	}
	return data, nil
}

func (c *RoborockApiClient) CodeLogin(ctx context.Context, code string) (map[string]any, error) {
	base, err := c.baseURLOrLogin(ctx)
	if err != nil {
		return nil, err
	}
	headers := map[string]string{"header_clientid": c.headerClientID()}
	resp, err := c.doRequest(ctx, "POST", base+"/api/v1/loginWithCode", map[string]string{
		"username":       c.username,
		"verifycode":     code,
		"verifycodetype": "AUTH_EMAIL_CODE",
	}, headers, nil)
	if err != nil {
		return nil, err
	}
	if int(asFloat(resp["code"])) != 200 {
		return nil, fmt.Errorf("login failed: %v", resp["msg"])
	}
	data, _ := resp["data"].(map[string]any)
	if data == nil {
		return nil, errors.New("missing user data")
	}
	return data, nil
}

func (c *RoborockApiClient) GetHomeID(ctx context.Context, userData *UserData) (int64, error) {
	base, err := c.baseURLOrLogin(ctx)
	if err != nil {
		return 0, err
	}
	headers := map[string]string{
		"header_clientid": c.headerClientID(),
		"Authorization":   userData.Token,
	}
	resp, err := c.doRequest(ctx, "GET", base+"/api/v1/getHomeDetail", nil, headers, nil)
	if err != nil {
		return 0, err
	}
	if int(asFloat(resp["code"])) != 200 {
		return 0, fmt.Errorf("get home id failed: %v", resp["msg"])
	}
	data, _ := resp["data"].(map[string]any)
	if data == nil {
		return 0, errors.New("missing home id")
	}
	return int64(asFloat(data["rrHomeId"])), nil
}

func (c *RoborockApiClient) GetHomeDataV3(ctx context.Context, userData *UserData) (*HomeData, error) {
	if userData == nil {
		return nil, errors.New("userData required")
	}
	homeID, err := c.GetHomeID(ctx, userData)
	if err != nil {
		return nil, err
	}
	base := userData.RRIOT.R.A
	if base == "" {
		return nil, errors.New("missing rriot base url")
	}
	headers := map[string]string{
		"Authorization": hawkAuth(userData.RRIOT, fmt.Sprintf("/v3/user/homes/%d", homeID), nil, nil),
	}
	resp, err := c.doRequest(ctx, "GET", base+fmt.Sprintf("/v3/user/homes/%d", homeID), nil, headers, nil)
	if err != nil {
		return nil, err
	}
	if success, _ := resp["success"].(bool); !success {
		return nil, fmt.Errorf("home data failed: %v", resp)
	}
	resultBytes, err := json.Marshal(resp["result"])
	if err != nil {
		return nil, err
	}
	var home HomeData
	if err := json.Unmarshal(resultBytes, &home); err != nil {
		return nil, err
	}
	return &home, nil
}

func (c *RoborockApiClient) headerClientID() string {
	md5sum := md5Bytes(append([]byte(c.username), []byte(c.deviceID)...))
	return base64.StdEncoding.EncodeToString(md5sum)
}

func (c *RoborockApiClient) doRequest(ctx context.Context, method, rawURL string, params map[string]string, headers map[string]string, form map[string]string) (map[string]any, error) {
	urlObj, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if params != nil {
		q := urlObj.Query()
		for k, v := range params {
			q.Set(k, v)
		}
		urlObj.RawQuery = q.Encode()
	}

	var body io.Reader
	if form != nil {
		vals := url.Values{}
		for k, v := range form {
			vals.Set(k, v)
		}
		body = strings.NewReader(vals.Encode())
	}

	req, err := http.NewRequestWithContext(ctx, method, urlObj.String(), body)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func randomAlphaNumeric(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = letters[rand.Intn(len(letters))]
	}
	return string(buf)
}

func asFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case int:
		return float64(t)
	case int64:
		return float64(t)
	case string:
		f, _ := strconv.ParseFloat(t, 64)
		return f
	default:
		return math.NaN()
	}
}

func hawkAuth(rriot RRiot, urlPath string, formdata map[string]string, params map[string]string) string {
	ts := time.Now().Unix()
	nonce := randomAlphaNumeric(8)
	paramsStr := hawkExtra(params)
	formStr := hawkExtra(formdata)
	prestr := strings.Join([]string{
		rriot.U,
		rriot.S,
		nonce,
		strconv.FormatInt(ts, 10),
		md5Hex([]byte(urlPath)),
		paramsStr,
		formStr,
	}, ":")
	mac := base64.StdEncoding.EncodeToString(hmacSha256([]byte(rriot.H), []byte(prestr)))
	return fmt.Sprintf("Hawk id=\"%s\",s=\"%s\",ts=\"%d\",nonce=\"%s\",mac=\"%s\"", rriot.U, rriot.S, ts, nonce, mac)
}

func hawkExtra(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, values[k]))
	}
	return md5Hex([]byte(strings.Join(parts, "&")))
}

func hmacSha256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write(data)
	return h.Sum(nil)
}
