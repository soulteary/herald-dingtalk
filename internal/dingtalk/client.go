package dingtalk

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	baseURL     = "https://oapi.dingtalk.com"
	getTokenURL = baseURL + "/gettoken"
	sendMsgURL  = baseURL + "/topapi/message/corpconversation/asyncsend_v2"
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
	return &Client{
		appKey:    appKey,
		appSecret: appSecret,
		agentID:   agentID,
		http:      &http.Client{Timeout: 15 * time.Second},
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
	defer resp.Body.Close()
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
	defer resp.Body.Close()
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
