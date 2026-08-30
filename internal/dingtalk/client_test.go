package dingtalk

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// redirectTransport forwards all requests to the given server (for testing).
type redirectTransport struct {
	base *httptest.Server
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type failingReadCloser struct{}

func (failingReadCloser) Read([]byte) (int, error) { return 0, errors.New("read failed") }
func (failingReadCloser) Close() error             { return nil }

func (r *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u := *req.URL
	u.Scheme = "http"
	u.Host = r.base.Listener.Addr().String()
	req2 := req.Clone(req.Context())
	req2.URL = &u
	return http.DefaultTransport.RoundTrip(req2)
}

func TestResolveAuthCode_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.0/oauth2/userAccessToken":
			if r.Method != http.MethodPost {
				t.Errorf("userAccessToken: want POST, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"accessToken":  "mock-token",
				"refreshToken": "mock-refresh",
				"expireIn":     7200,
				"corpId":       "corp1",
			})
		case "/v1.0/contact/users/me":
			if r.Method != http.MethodGet || r.Header.Get("x-acs-dingtalk-access-token") != "mock-token" {
				t.Errorf("users/me: bad request")
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"userId":  "user123",
				"unionId": "union1",
				"nick":    "nick",
			})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClientWithHTTP("key", "secret", "1", &http.Client{
		Transport: &redirectTransport{base: server},
	})
	userid, err := client.ResolveAuthCode(context.Background(), "auth-code-xyz")
	if err != nil {
		t.Fatalf("ResolveAuthCode: %v", err)
	}
	if userid != "user123" {
		t.Errorf("userid = %q, want user123", userid)
	}
}

func TestResolveAuthCode_TokenAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1.0/oauth2/userAccessToken" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "InvalidCode", "message": "code expired"})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClientWithHTTP("key", "secret", "1", &http.Client{
		Transport: &redirectTransport{base: server},
	})
	_, err := client.ResolveAuthCode(context.Background(), "bad-code")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "oauth2 userAccessToken: code expired" {
		t.Errorf("err = %v", err)
	}
}

func TestResolveAuthCode_MeEmptyUserId(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.0/oauth2/userAccessToken":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "t", "expireIn": 7200})
		case "/v1.0/contact/users/me":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"userId": "", "unionId": "u1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClientWithHTTP("key", "secret", "1", &http.Client{
		Transport: &redirectTransport{base: server},
	})
	_, err := client.ResolveAuthCode(context.Background(), "code")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "oauth2 users/me: empty userId" {
		t.Errorf("err = %v", err)
	}
}

func TestGetUserIDByMobile_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gettoken":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "tok", "expires_in": 7200})
		case "/topapi/v2/user/getbymobile":
			if r.URL.Query().Get("mobile") != "13800138000" {
				t.Errorf("mobile = %s", r.URL.Query().Get("mobile"))
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"errcode": 0,
				"errmsg":  "ok",
				"result":  map[string]any{"userid": "uid-by-mobile"},
			})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClientWithHTTP("key", "secret", "1", &http.Client{
		Transport: &redirectTransport{base: server},
	})
	userid, err := client.GetUserIDByMobile(context.Background(), "13800138000")
	if err != nil {
		t.Fatalf("GetUserIDByMobile: %v", err)
	}
	if userid != "uid-by-mobile" {
		t.Errorf("userid = %q, want uid-by-mobile", userid)
	}
}

func TestGetUserIDByMobile_ErrcodeNonZero(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gettoken" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "tok", "expires_in": 7200})
			return
		}
		if r.URL.Path == "/topapi/v2/user/getbymobile" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 60011, "errmsg": "user not found"})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClientWithHTTP("key", "secret", "1", &http.Client{
		Transport: &redirectTransport{base: server},
	})
	_, err := client.GetUserIDByMobile(context.Background(), "13900000000")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "getbymobile: errcode=60011 errmsg=user not found" {
		t.Errorf("err = %v", err)
	}
}

func TestGetUserIDByMobile_EmptyUserid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gettoken" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "tok", "expires_in": 7200})
			return
		}
		if r.URL.Path == "/topapi/v2/user/getbymobile" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "errmsg": "ok", "result": map[string]any{"userid": ""}})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewClientWithHTTP("key", "secret", "1", &http.Client{
		Transport: &redirectTransport{base: server},
	})
	_, err := client.GetUserIDByMobile(context.Background(), "13800138000")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "getbymobile: no userid for mobile" {
		t.Errorf("err = %v", err)
	}
	if category := ClassifyError(err); category != ErrorCategoryInvalidDestination {
		t.Errorf("category = %q, want %q", category, ErrorCategoryInvalidDestination)
	}
}

func TestSendWorkNotify_InvalidAgentID(t *testing.T) {
	client := NewClientWithHTTP("key", "secret", "not-a-number", &http.Client{})
	_, err := client.SendWorkNotify(context.Background(), "user", "hello")
	if err == nil || !strings.Contains(err.Error(), "invalid DingTalk agent ID") {
		t.Fatalf("err = %v, want invalid agent ID", err)
	}
}

func TestSendWorkNotify_RefreshesRejectedTokenOnce(t *testing.T) {
	var tokenCalls atomic.Int32
	var sendCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gettoken":
			call := tokenCalls.Add(1)
			token := "old-token"
			if call == 2 {
				token = "new-token"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"errcode": 0, "access_token": token, "expires_in": 7200,
			})
		case "/topapi/message/corpconversation/asyncsend_v2":
			sendCalls.Add(1)
			if r.URL.Query().Get("access_token") == "old-token" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"errcode": invalidTokenCode, "errmsg": "invalid token",
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "task_id": 123})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClientWithHTTP("key", "secret", "1", &http.Client{
		Transport: &redirectTransport{base: server},
	})
	taskID, err := client.SendWorkNotify(context.Background(), "user", "hello")
	if err != nil {
		t.Fatalf("SendWorkNotify: %v", err)
	}
	if taskID != "123" || tokenCalls.Load() != 2 || sendCalls.Load() != 2 {
		t.Fatalf("taskID=%q tokenCalls=%d sendCalls=%d", taskID, tokenCalls.Load(), sendCalls.Load())
	}
}

func TestGetToken_CoalescesConcurrentRefresh(t *testing.T) {
	var tokenCalls atomic.Int32
	tokenStarted := make(chan struct{})
	releaseToken := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gettoken":
			if tokenCalls.Add(1) == 1 {
				close(tokenStarted)
			}
			<-releaseToken
			_ = json.NewEncoder(w).Encode(map[string]any{
				"errcode": 0, "access_token": "shared-token", "expires_in": 7200,
			})
		case "/topapi/v2/user/getbymobile":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"errcode": 0, "result": map[string]any{"userid": "user"},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClientWithHTTP("key", "secret", "1", &http.Client{
		Transport: &redirectTransport{base: server},
	})
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := client.GetUserIDByMobile(context.Background(), "13800138000")
			errs <- err
		}()
	}
	<-tokenStarted
	time.Sleep(20 * time.Millisecond)
	close(releaseToken)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("GetUserIDByMobile: %v", err)
		}
	}
	if tokenCalls.Load() != 1 {
		t.Fatalf("token calls = %d, want 1", tokenCalls.Load())
	}
}

func TestClientRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(strings.Repeat("x", int(maxResponseBytes)+1)))
	}))
	defer server.Close()

	client := NewClientWithHTTP("key", "secret", "1", &http.Client{
		Transport: &redirectTransport{base: server},
	})
	_, err := client.GetUserIDByMobile(context.Background(), "13800138000")
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("err = %v, want ErrResponseTooLarge", err)
	}
}

func TestClientRejectsNon2xxResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("bad gateway"))
	}))
	defer server.Close()

	client := NewClientWithHTTP("key", "secret", "1", &http.Client{
		Transport: &redirectTransport{base: server},
	})
	_, err := client.GetUserIDByMobile(context.Background(), "13800138000")
	var statusErr *httpStatusError
	if !errors.As(err, &statusErr) || statusErr.status != http.StatusBadGateway {
		t.Fatalf("err = %v, want HTTP 502 status error", err)
	}
}

func TestGetTokenValidatesRequiredFieldsAndEncodesCredentials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("appsecret") != "secret+with&symbols" {
			t.Errorf("appsecret = %q", r.URL.Query().Get("appsecret"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"errcode": 0, "access_token": "", "expires_in": 0,
		})
	}))
	defer server.Close()

	client := NewClientWithHTTP("key", "secret+with&symbols", "1", &http.Client{
		Transport: &redirectTransport{base: server},
	})
	_, err := client.GetUserIDByMobile(context.Background(), "13800138000")
	if err == nil || err.Error() != "dingtalk gettoken: empty access_token" {
		t.Fatalf("err = %v", err)
	}
}

func TestSendWorkNotifyRejectsEmptyTaskID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gettoken":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"errcode": 0, "access_token": "token", "expires_in": 7200,
			})
		case "/topapi/message/corpconversation/asyncsend_v2":
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "task_id": 0})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewClientWithHTTP("key", "secret", "1", &http.Client{
		Transport: &redirectTransport{base: server},
	})
	_, err := client.SendWorkNotify(context.Background(), "user", "hello")
	if err == nil || err.Error() != "dingtalk send: empty task_id" {
		t.Fatalf("err = %v", err)
	}
}

func TestNewClientUsesSafeHTTPDefaults(t *testing.T) {
	client := NewClient("key", "secret", "1")
	if client.http == nil {
		t.Fatal("default HTTP client is nil")
	}
	if client.http.Timeout != defaultClientTimeout {
		t.Fatalf("timeout = %s, want %s", client.http.Timeout, defaultClientTimeout)
	}
	if client.http.CheckRedirect == nil {
		t.Fatal("redirect policy is not configured")
	}
	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.http.CheckRedirect(req, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("redirect error = %v, want http.ErrUseLastResponse", err)
	}
}

func TestHTTPStatusErrorMessage(t *testing.T) {
	err := (&httpStatusError{operation: "getbymobile", status: http.StatusBadGateway}).Error()
	if err != "getbymobile: HTTP 502" {
		t.Fatalf("error = %q", err)
	}
}

func TestClassifyError(t *testing.T) {
	for _, tt := range []struct {
		name string
		err  error
		want ErrorCategory
	}{
		{name: "nil", err: nil, want: ""},
		{name: "timeout", err: context.DeadlineExceeded, want: ErrorCategoryTimeout},
		{name: "invalid destination", err: &apiError{code: 60121}, want: ErrorCategoryInvalidDestination},
		{name: "invalid token", err: &apiError{code: invalidTokenCode}, want: ErrorCategoryProviderDown},
		{name: "api upstream", err: &apiError{code: 500}, want: ErrorCategoryUpstream},
		{name: "http unauthorized", err: &httpStatusError{status: http.StatusUnauthorized}, want: ErrorCategoryProviderDown},
		{name: "http rate limited", err: &httpStatusError{status: http.StatusTooManyRequests}, want: ErrorCategoryRateLimited},
		{name: "http upstream", err: &httpStatusError{status: http.StatusBadGateway}, want: ErrorCategoryUpstream},
		{name: "explicit category", err: withCategory(ErrorCategoryInvalidRequest, errors.New("bad code")), want: ErrorCategoryInvalidRequest},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyError(tt.err); got != tt.want {
				t.Fatalf("ClassifyError(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

func TestGetTokenErrorResponses(t *testing.T) {
	for _, tt := range []struct {
		name string
		body string
		want string
	}{
		{name: "invalid JSON", body: "{", want: "dingtalk gettoken parse"},
		{name: "API error", body: `{"errcode":123,"errmsg":"denied"}`, want: "errcode=123"},
		{name: "invalid expiry", body: `{"errcode":0,"access_token":"token","expires_in":0}`, want: "invalid expires_in"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()
			client := NewClientWithHTTP("key", "secret", "1", &http.Client{Transport: &redirectTransport{base: server}})
			_, err := client.getToken(context.Background())
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestGetTokenUsesCachedToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gettoken" {
			t.Fatal("cached token must avoid refresh")
		}
		if got := r.URL.Query().Get("access_token"); got != "cached-token" {
			t.Errorf("access_token = %q, want cached-token", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "result": map[string]any{"userid": "user"}})
	}))
	defer server.Close()
	client := NewClientWithHTTP("key", "secret", "1", &http.Client{Transport: &redirectTransport{base: server}})
	client.token = "cached-token"
	client.expires = time.Now().Add(time.Minute)
	userid, err := client.GetUserIDByMobile(context.Background(), "13800138000")
	if err != nil || userid != "user" {
		t.Fatalf("userid = %q, err = %v", userid, err)
	}
}

func TestGetTokenWaiterHonorsCancellation(t *testing.T) {
	client := NewClientWithHTTP("key", "secret", "1", &http.Client{})
	client.refresh = &tokenRefresh{done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.getToken(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}


func TestGetTokenWaiterRetriesAfterLeaderCancellation(t *testing.T) {
	leaderStarted := make(chan struct{})
	var calls atomic.Int32
	client := NewClientWithHTTP("key", "secret", "1", &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(leaderStarted)
			<-req.Context().Done()
			return nil, req.Context().Err()
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`{"errcode":0,"access_token":"retry-token","expires_in":7200}`)),
			Request:    req,
		}, nil
	})})

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderDone := make(chan error, 1)
	go func() {
		_, err := client.getToken(leaderCtx)
		leaderDone <- err
	}()
	<-leaderStarted

	waiterDone := make(chan struct {
		token string
		err   error
	}, 1)
	go func() {
		token, err := client.getToken(context.Background())
		waiterDone <- struct {
			token string
			err   error
		}{token, err}
	}()

	cancelLeader()
	if err := <-leaderDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader error = %v, want context.Canceled", err)
	}
	waiter := <-waiterDone
	if waiter.err != nil || waiter.token != "retry-token" {
		t.Fatalf("waiter = %q, %v", waiter.token, waiter.err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("refresh calls = %d, want 2", got)
	}
}

func TestClientDoPropagatesTransportAndReadErrors(t *testing.T) {
	transportErr := errors.New("transport failed")
	client := NewClientWithHTTP("key", "secret", "1", &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, transportErr
	})})
	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.do(req, "test"); !errors.Is(err, transportErr) {
		t.Fatalf("transport error = %v", err)
	}

	client = NewClientWithHTTP("key", "secret", "1", &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: failingReadCloser{}, Header: make(http.Header)}, nil
	})})
	if _, err := client.do(req, "test"); err == nil || err.Error() != "read failed" {
		t.Fatalf("read error = %v", err)
	}
}

func TestResponseMessageErrorFallbacks(t *testing.T) {
	if got := responseMessageError("operation", []byte(`{"code":"BadCode"}`), "failed").Error(); got != "operation: failed (code=BadCode)" {
		t.Fatalf("code error = %q", got)
	}
	if got := responseMessageError("operation", []byte("not-json"), "failed").Error(); got != "operation: failed" {
		t.Fatalf("fallback error = %q", got)
	}
	base := errors.New("network down")
	if got := oauthError("oauth", base); !errors.Is(got, base) {
		t.Fatalf("oauth error = %v, want wrapped base error", got)
	}
}

func TestResolveAuthCodeRejectsMalformedResponses(t *testing.T) {
	for _, tt := range []struct {
		name      string
		tokenBody string
		userBody  string
		want      string
	}{
		{name: "token JSON", tokenBody: "{", want: "userAccessToken parse"},
		{name: "user JSON", tokenBody: `{"accessToken":"token"}`, userBody: "{", want: "users/me parse"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1.0/oauth2/userAccessToken" {
					_, _ = w.Write([]byte(tt.tokenBody))
					return
				}
				_, _ = w.Write([]byte(tt.userBody))
			}))
			defer server.Close()
			client := NewClientWithHTTP("key", "secret", "1", &http.Client{Transport: &redirectTransport{base: server}})
			_, err := client.ResolveAuthCode(context.Background(), "code")
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}

func TestSendWorkNotifyRejectsInvalidUser(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gettoken":
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "token", "expires_in": 7200})
		case "/topapi/message/corpconversation/asyncsend_v2":
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "task_id": 1, "invalid_user": "missing-user"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClientWithHTTP("key", "secret", "1", &http.Client{Transport: &redirectTransport{base: server}})
	_, err := client.SendWorkNotify(context.Background(), "missing-user", "hello")
	if err == nil || !strings.Contains(err.Error(), "invalid_user=missing-user") {
		t.Fatalf("error = %v", err)
	}
}

func TestGetUserIDByMobileRejectsMalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/gettoken" {
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "token", "expires_in": 7200})
			return
		}
		_, _ = io.WriteString(w, "{")
	}))
	defer server.Close()
	client := NewClientWithHTTP("key", "secret", "1", &http.Client{Transport: &redirectTransport{base: server}})
	_, err := client.GetUserIDByMobile(context.Background(), "13800138000")
	if err == nil || !strings.Contains(err.Error(), "getbymobile parse") {
		t.Fatalf("error = %v", err)
	}
}
