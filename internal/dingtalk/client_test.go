package dingtalk

import (
	"context"
	"encoding/json"
	"errors"
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
	req := httptest.NewRequest(http.MethodGet, "https://example.com", nil)
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
