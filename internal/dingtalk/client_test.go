package dingtalk

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestMustParseInt64(t *testing.T) {
	if got := mustParseInt64("123"); got != 123 {
		t.Errorf("mustParseInt64(123) = %d, want 123", got)
	}
	if got := mustParseInt64("0"); got != 0 {
		t.Errorf("mustParseInt64(0) = %d", got)
	}
}
