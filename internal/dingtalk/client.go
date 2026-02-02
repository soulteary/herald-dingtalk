package dingtalk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"
)

const (
	baseURL       = "https://oapi.dingtalk.com"
	oauth2BaseURL = "https://api.dingtalk.com"
	getTokenURL   = baseURL + "/gettoken"
	sendMsgURL    = baseURL + "/topapi/message/corpconversation/asyncsend_v2"
	// OAuth2: 授权码换 userAccessToken，再取用户信息得 userid
	oauth2UserTokenURL = oauth2BaseURL + "/v1.0/oauth2/userAccessToken"
	oauth2UserMeURL    = oauth2BaseURL + "/v1.0/contact/users/me"
	getByMobileURL     = baseURL + "/topapi/v2/user/getbymobile"
)

type tokenResp struct {
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
}

type sendReq struct {
	AgentID    int64       `json:"agent_id"`
	UserIDList string      `json:"userid_list"`
	Msg        interface{} `json:"msg"`
}

type textMsg struct {
	MsgType string   `json:"msgtype"`
	Text    textBody `json:"text"`
}

type textBody struct {
	Content string `json:"content"`
}

type sendResp struct {
	ErrCode     int    `json:"errcode"`
	ErrMsg      string `json:"errmsg"`
	TaskID      int64  `json:"task_id"`
	RequestID   string `json:"request_id"`
	InvalidUser string `json:"invalid_user"`
}

// OAuth2 userAccessToken 请求/响应（api.dingtalk.com）
type oauth2UserTokenReq struct {
	ClientID     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	Code         string `json:"code"`
	GrantType    string `json:"grantType"`
}

type oauth2UserTokenResp struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ExpireIn     int    `json:"expireIn"`
	CorpID       string `json:"corpId"`
}

type oauth2UserMeResp struct {
	UserID  string `json:"userId"`
	UnionID string `json:"unionId"`
	Nick    string `json:"nick"`
	Avatar  string `json:"avatarUrl"`
}

// getByMobile 响应（oapi）
type getByMobileResp struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
	Result  struct {
		UserID string `json:"userid"`
	} `json:"result"`
}

// Client calls DingTalk work notification API.
type Client struct {
	appKey    string
	appSecret string
	agentID   string
	http      *http.Client
	mu        sync.Mutex
	token     string
	expires   time.Time
}

// NewClient creates a DingTalk API client.
func NewClient(appKey, appSecret, agentID string) *Client {
	return NewClientWithHTTP(appKey, appSecret, agentID, nil)
}

// NewClientWithHTTP creates a DingTalk API client with a custom *http.Client (e.g. for tests).
// If httpClient is nil, a default client with 15s timeout is used.
func NewClientWithHTTP(appKey, appSecret, agentID string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{
		appKey:    appKey,
		appSecret: appSecret,
		agentID:   agentID,
		http:      httpClient,
	}
}

// getToken returns a valid access token, refreshing if needed.
func (c *Client) getToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	if c.token != "" && time.Now().Before(c.expires) {
		tok := c.token
		c.mu.Unlock()
		return tok, nil
	}
	c.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s?appkey=%s&appsecret=%s", getTokenURL, c.appKey, c.appSecret), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var tr tokenResp
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", err
	}
	if tr.ErrCode != 0 {
		return "", fmt.Errorf("dingtalk gettoken: errcode=%d errmsg=%s", tr.ErrCode, tr.ErrMsg)
	}
	c.mu.Lock()
	c.token = tr.AccessToken
	c.expires = time.Now().Add(time.Duration(tr.ExpiresIn-120) * time.Second)
	c.mu.Unlock()
	return tr.AccessToken, nil
}

// SendWorkNotify sends a text work notification to the given userid.
// userid is DingTalk user ID (single user); content is the message body.
func (c *Client) SendWorkNotify(ctx context.Context, userid, content string) (taskID string, err error) {
	tok, err := c.getToken(ctx)
	if err != nil {
		return "", err
	}
	msg := sendReq{
		AgentID:    mustParseInt64(c.agentID),
		UserIDList: userid,
		Msg: textMsg{
			MsgType: "text",
			Text:    textBody{Content: content},
		},
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sendMsgURL+"?access_token="+tok, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var sr sendResp
	if err := json.Unmarshal(respBody, &sr); err != nil {
		return "", err
	}
	if sr.ErrCode != 0 {
		return "", fmt.Errorf("dingtalk send: errcode=%d errmsg=%s", sr.ErrCode, sr.ErrMsg)
	}
	return fmt.Sprintf("%d", sr.TaskID), nil
}

func mustParseInt64(s string) int64 {
	var i int64
	_, _ = fmt.Sscanf(s, "%d", &i)
	return i
}

// ResolveAuthCode exchanges OAuth2 auth_code for userid via userAccessToken + users/me.
// See: https://open.dingtalk.com/document/orgapp/obtain-identity-credentials
func (c *Client) ResolveAuthCode(ctx context.Context, code string) (userid string, err error) {
	body := oauth2UserTokenReq{
		ClientID:     c.appKey,
		ClientSecret: c.appSecret,
		Code:         code,
		GrantType:    "authorization_code",
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauth2UserTokenURL, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var tr oauth2UserTokenResp
	if err := json.Unmarshal(respBody, &tr); err != nil {
		return "", fmt.Errorf("oauth2 userAccessToken parse: %w", err)
	}
	if tr.AccessToken == "" {
		var errResp struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(respBody, &errResp)
		if errResp.Message != "" {
			return "", fmt.Errorf("oauth2 userAccessToken: %s", errResp.Message)
		}
		return "", fmt.Errorf("oauth2 userAccessToken: empty access_token (code=%s)", errResp.Code)
	}
	// GET /v1.0/contact/users/me
	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, oauth2UserMeURL, nil)
	if err != nil {
		return "", err
	}
	req2.Header.Set("x-acs-dingtalk-access-token", tr.AccessToken)
	resp2, err := c.http.Do(req2)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp2.Body.Close() }()
	meBody, err := io.ReadAll(resp2.Body)
	if err != nil {
		return "", err
	}
	var me oauth2UserMeResp
	if err := json.Unmarshal(meBody, &me); err != nil {
		return "", fmt.Errorf("oauth2 users/me parse: %w", err)
	}
	if me.UserID == "" {
		var errResp struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(meBody, &errResp)
		if errResp.Message != "" {
			return "", fmt.Errorf("oauth2 users/me: %s", errResp.Message)
		}
		return "", fmt.Errorf("oauth2 users/me: empty userId")
	}
	return me.UserID, nil
}

// GetUserIDByMobile returns userid for the given mobile using topapi/v2/user/getbymobile.
// Requires Contact.User.mobile permission. See: https://open.dingtalk.com/document/orgapp-server/query-users-by-phone-number
func (c *Client) GetUserIDByMobile(ctx context.Context, mobile string) (userid string, err error) {
	tok, err := c.getToken(ctx)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		getByMobileURL+"?access_token="+url.QueryEscape(tok)+"&mobile="+url.QueryEscape(mobile), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var gr getByMobileResp
	if err := json.Unmarshal(body, &gr); err != nil {
		return "", err
	}
	if gr.ErrCode != 0 {
		return "", fmt.Errorf("getbymobile: errcode=%d errmsg=%s", gr.ErrCode, gr.ErrMsg)
	}
	if gr.Result.UserID == "" {
		return "", fmt.Errorf("getbymobile: no userid for mobile")
	}
	return gr.Result.UserID, nil
}
