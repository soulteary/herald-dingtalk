package dingtalk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const (
	baseURL              = "https://oapi.dingtalk.com"
	oauth2BaseURL        = "https://api.dingtalk.com"
	getTokenURL          = baseURL + "/gettoken"
	sendMsgURL           = baseURL + "/topapi/message/corpconversation/asyncsend_v2"
	oauth2UserTokenURL   = oauth2BaseURL + "/v1.0/oauth2/userAccessToken"
	oauth2UserMeURL      = oauth2BaseURL + "/v1.0/contact/users/me"
	getByMobileURL       = baseURL + "/topapi/v2/user/getbymobile"
	maxResponseBytes     = int64(1 << 20)
	invalidTokenCode     = 40014
	expiredTokenCode     = 42001
	defaultClientTimeout = 15 * time.Second
)

var ErrResponseTooLarge = errors.New("dingtalk response exceeds 1 MiB")

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

type getByMobileResp struct {
	ErrCode int    `json:"errcode"`
	ErrMsg  string `json:"errmsg"`
	Result  struct {
		UserID string `json:"userid"`
	} `json:"result"`
}

type apiError struct {
	operation string
	code      int
	message   string
}

// ErrorCategory describes the stable failure classes handlers expose without
// leaking raw DingTalk responses to callers.
type ErrorCategory string

const (
	ErrorCategoryUpstream           ErrorCategory = "upstream"
	ErrorCategoryInvalidRequest     ErrorCategory = "invalid_request"
	ErrorCategoryInvalidDestination ErrorCategory = "invalid_destination"
	ErrorCategoryRateLimited        ErrorCategory = "rate_limited"
	ErrorCategoryProviderDown       ErrorCategory = "provider_down"
	ErrorCategoryTimeout            ErrorCategory = "timeout"
)

type classifiedError struct {
	category ErrorCategory
	err      error
}

func (e *classifiedError) Error() string { return e.err.Error() }
func (e *classifiedError) Unwrap() error { return e.err }

func withCategory(category ErrorCategory, err error) error {
	if err == nil {
		return nil
	}
	return &classifiedError{category: category, err: err}
}

// ClassifyError converts transport, HTTP, and DingTalk API failures into a
// stable category suitable for HTTP status mapping.
func ClassifyError(err error) ErrorCategory {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrorCategoryTimeout
	}
	var classified *classifiedError
	if errors.As(err, &classified) {
		return classified.category
	}
	var apiErr *apiError
	if errors.As(err, &apiErr) {
		switch apiErr.code {
		case 60011, 60121, 40104:
			return ErrorCategoryInvalidDestination
		case 40013, invalidTokenCode, expiredTokenCode:
			return ErrorCategoryProviderDown
		default:
			return ErrorCategoryUpstream
		}
	}
	var statusErr *httpStatusError
	if errors.As(err, &statusErr) {
		switch statusErr.status {
		case http.StatusUnauthorized, http.StatusForbidden:
			return ErrorCategoryProviderDown
		case http.StatusTooManyRequests:
			return ErrorCategoryRateLimited
		default:
			return ErrorCategoryUpstream
		}
	}
	return ErrorCategoryUpstream
}

func (e *apiError) Error() string {
	return fmt.Sprintf("%s: errcode=%d errmsg=%s", e.operation, e.code, e.message)
}

func (e *apiError) tokenInvalid() bool {
	return e.code == invalidTokenCode || e.code == expiredTokenCode
}

type httpStatusError struct {
	operation string
	status    int
	body      []byte
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("%s: HTTP %d", e.operation, e.status)
}

type tokenRefresh struct {
	done           chan struct{}
	token          string
	err            error
	leaderCanceled bool
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
	refresh   *tokenRefresh
}

// NewClient creates a DingTalk API client.
func NewClient(appKey, appSecret, agentID string) *Client {
	return NewClientWithHTTP(appKey, appSecret, agentID, nil)
}

// NewClientWithHTTP creates a DingTalk API client with a custom *http.Client (e.g. for tests).
func NewClientWithHTTP(appKey, appSecret, agentID string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: defaultClientTimeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &Client{
		appKey: appKey, appSecret: appSecret, agentID: agentID, http: httpClient,
	}
}

// getToken returns a valid access token and coalesces concurrent refreshes.
func (c *Client) getToken(ctx context.Context) (string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		c.mu.Lock()
		if c.token != "" && time.Now().Before(c.expires) {
			token := c.token
			c.mu.Unlock()
			return token, nil
		}
		if c.refresh != nil {
			active := c.refresh
			c.mu.Unlock()
			select {
			case <-active.done:
				// A shared refresh is owned by its leader's context. If only that
				// leader was canceled, a live waiter must be allowed to retry.
				if active.leaderCanceled && ctx.Err() == nil {
					continue
				}
				return active.token, active.err
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		active := &tokenRefresh{done: make(chan struct{})}
		c.refresh = active
		c.mu.Unlock()

		token, expires, err := c.fetchToken(ctx)
		c.mu.Lock()
		if err == nil {
			c.token = token
			c.expires = expires
		}
		active.token = token
		active.err = err
		active.leaderCanceled = ctx.Err() != nil
		c.refresh = nil
		close(active.done)
		c.mu.Unlock()
		return token, err
	}
}

func (c *Client) fetchToken(ctx context.Context) (string, time.Time, error) {
	query := url.Values{}
	query.Set("appkey", c.appKey)
	query.Set("appsecret", c.appSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, getTokenURL+"?"+query.Encode(), nil)
	if err != nil {
		return "", time.Time{}, err
	}
	body, err := c.do(req, "dingtalk gettoken")
	if err != nil {
		return "", time.Time{}, err
	}
	var response tokenResp
	if err := json.Unmarshal(body, &response); err != nil {
		return "", time.Time{}, fmt.Errorf("dingtalk gettoken parse: %w", err)
	}
	if response.ErrCode != 0 {
		return "", time.Time{}, &apiError{operation: "dingtalk gettoken", code: response.ErrCode, message: response.ErrMsg}
	}
	if response.AccessToken == "" {
		return "", time.Time{}, errors.New("dingtalk gettoken: empty access_token")
	}
	if response.ExpiresIn <= 0 {
		return "", time.Time{}, errors.New("dingtalk gettoken: invalid expires_in")
	}
	lifetime := time.Duration(response.ExpiresIn) * time.Second
	skew := lifetime / 10
	if skew > 120*time.Second {
		skew = 120 * time.Second
	}
	return response.AccessToken, time.Now().Add(lifetime - skew), nil
}

// SendWorkNotify sends a text work notification to the given userid.
func (c *Client) SendWorkNotify(ctx context.Context, userid, content string) (string, error) {
	agentID, err := strconv.ParseInt(c.agentID, 10, 64)
	if err != nil || agentID <= 0 {
		return "", fmt.Errorf("invalid DingTalk agent ID %q", c.agentID)
	}
	for attempt := 0; attempt < 2; attempt++ {
		token, err := c.getToken(ctx)
		if err != nil {
			return "", err
		}
		taskID, err := c.sendWithToken(ctx, token, agentID, userid, content)
		var apiErr *apiError
		if attempt == 0 && errors.As(err, &apiErr) && apiErr.tokenInvalid() {
			c.invalidateToken(token)
			continue
		}
		return taskID, err
	}
	return "", errors.New("dingtalk send: token refresh retry exhausted")
}

func (c *Client) sendWithToken(ctx context.Context, token string, agentID int64, userid, content string) (string, error) {
	message := sendReq{
		AgentID: agentID, UserIDList: userid,
		Msg: textMsg{MsgType: "text", Text: textBody{Content: content}},
	}
	raw, err := json.Marshal(message)
	if err != nil {
		return "", err
	}
	query := url.Values{}
	query.Set("access_token", token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sendMsgURL+"?"+query.Encode(), bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	body, err := c.do(req, "dingtalk send")
	if err != nil {
		return "", err
	}
	var response sendResp
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("dingtalk send parse: %w", err)
	}
	if response.ErrCode != 0 {
		return "", &apiError{operation: "dingtalk send", code: response.ErrCode, message: response.ErrMsg}
	}
	if response.InvalidUser != "" {
		return "", fmt.Errorf("dingtalk send: invalid_user=%s", response.InvalidUser)
	}
	if response.TaskID <= 0 {
		return "", errors.New("dingtalk send: empty task_id")
	}
	return strconv.FormatInt(response.TaskID, 10), nil
}

// ResolveAuthCode exchanges OAuth2 auth_code for userid via userAccessToken + users/me.
func (c *Client) ResolveAuthCode(ctx context.Context, code string) (string, error) {
	payload := oauth2UserTokenReq{
		ClientID: c.appKey, ClientSecret: c.appSecret, Code: code, GrantType: "authorization_code",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, oauth2UserTokenURL, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	body, err := c.do(req, "oauth2 userAccessToken")
	if err != nil {
		return "", oauthError("oauth2 userAccessToken", err)
	}
	var token oauth2UserTokenResp
	if err := json.Unmarshal(body, &token); err != nil {
		return "", fmt.Errorf("oauth2 userAccessToken parse: %w", err)
	}
	if token.AccessToken == "" {
		return "", withCategory(ErrorCategoryInvalidRequest, responseMessageError("oauth2 userAccessToken", body, "empty access_token"))
	}

	req, err = http.NewRequestWithContext(ctx, http.MethodGet, oauth2UserMeURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("x-acs-dingtalk-access-token", token.AccessToken)
	body, err = c.do(req, "oauth2 users/me")
	if err != nil {
		return "", oauthError("oauth2 users/me", err)
	}
	var user oauth2UserMeResp
	if err := json.Unmarshal(body, &user); err != nil {
		return "", fmt.Errorf("oauth2 users/me parse: %w", err)
	}
	if user.UserID == "" {
		return "", responseMessageError("oauth2 users/me", body, "empty userId")
	}
	return user.UserID, nil
}

// GetUserIDByMobile returns userid for the given mobile.
func (c *Client) GetUserIDByMobile(ctx context.Context, mobile string) (string, error) {
	for attempt := 0; attempt < 2; attempt++ {
		token, err := c.getToken(ctx)
		if err != nil {
			return "", err
		}
		userid, err := c.getUserIDByMobileWithToken(ctx, token, mobile)
		var apiErr *apiError
		if attempt == 0 && errors.As(err, &apiErr) && apiErr.tokenInvalid() {
			c.invalidateToken(token)
			continue
		}
		return userid, err
	}
	return "", errors.New("getbymobile: token refresh retry exhausted")
}

func (c *Client) getUserIDByMobileWithToken(ctx context.Context, token, mobile string) (string, error) {
	query := url.Values{}
	query.Set("access_token", token)
	query.Set("mobile", mobile)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, getByMobileURL+"?"+query.Encode(), nil)
	if err != nil {
		return "", err
	}
	body, err := c.do(req, "getbymobile")
	if err != nil {
		return "", err
	}
	var response getByMobileResp
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("getbymobile parse: %w", err)
	}
	if response.ErrCode != 0 {
		return "", &apiError{operation: "getbymobile", code: response.ErrCode, message: response.ErrMsg}
	}
	if response.Result.UserID == "" {
		return "", withCategory(ErrorCategoryInvalidDestination, errors.New("getbymobile: no userid for mobile"))
	}
	return response.Result.UserID, nil
}

func (c *Client) invalidateToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token == token {
		c.token = ""
		c.expires = time.Time{}
	}
}

func (c *Client) do(req *http.Request, operation string) ([]byte, error) {
	response, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxResponseBytes {
		return nil, ErrResponseTooLarge
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, &httpStatusError{operation: operation, status: response.StatusCode, body: body}
	}
	return body, nil
}

func oauthError(operation string, err error) error {
	var statusErr *httpStatusError
	if errors.As(err, &statusErr) {
		category := ErrorCategoryUpstream
		switch statusErr.status {
		case http.StatusBadRequest:
			category = ErrorCategoryInvalidRequest
		case http.StatusUnauthorized, http.StatusForbidden:
			category = ErrorCategoryProviderDown
		case http.StatusTooManyRequests:
			category = ErrorCategoryRateLimited
		}
		return withCategory(category, responseMessageError(operation, statusErr.body, fmt.Sprintf("HTTP %d", statusErr.status)))
	}
	return withCategory(ClassifyError(err), fmt.Errorf("%s: %w", operation, err))
}

func responseMessageError(operation string, body []byte, fallback string) error {
	var response struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &response)
	if response.Message != "" {
		return fmt.Errorf("%s: %s", operation, response.Message)
	}
	if response.Code != "" {
		return fmt.Errorf("%s: %s (code=%s)", operation, fallback, response.Code)
	}
	return fmt.Errorf("%s: %s", operation, fallback)
}
