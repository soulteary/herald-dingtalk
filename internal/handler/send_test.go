package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/soulteary/herald-dingtalk/internal/config"
	"github.com/soulteary/herald-dingtalk/internal/dingtalk"
	"github.com/soulteary/herald-dingtalk/internal/idempotency"
	"github.com/soulteary/logger-kit"
	"github.com/soulteary/provider-kit"
)

type waitForCancellationTransport struct{}

func (waitForCancellationTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func TestSendHandler_SuccessWithUserid(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gettoken":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "tok", "expires_in": 7200})
		case "/topapi/message/corpconversation/asyncsend_v2":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "task_id": 999})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := dingtalk.NewClientWithHTTP("k", "s", "1", &http.Client{Transport: &redirectTransport{base: server}})
	idemStore := idempotency.NewStore(300)
	log := logger.New(logger.Config{Level: logger.ErrorLevel})
	app := fiber.New()
	app.Post("/v1/send", func(c *fiber.Ctx) error { return SendHandler(c, client, idemStore, log) })

	body := bytes.NewBufferString(`{"to":"userid123","body":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/send", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var out struct {
		OK        bool   `json:"ok"`
		MessageID string `json:"message_id"`
		Provider  string `json:"provider"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.OK || out.MessageID != "999" || out.Provider != "dingtalk" {
		t.Errorf("ok=%v message_id=%q provider=%q", out.OK, out.MessageID, out.Provider)
	}
}

func TestSendHandler_EmptyTo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	client := dingtalk.NewClientWithHTTP("k", "s", "1", &http.Client{Transport: &redirectTransport{base: server}})
	idemStore := idempotency.NewStore(300)
	log := logger.New(logger.Config{Level: logger.ErrorLevel})
	app := fiber.New()
	app.Post("/v1/send", func(c *fiber.Ctx) error { return SendHandler(c, client, idemStore, log) })

	body := bytes.NewBufferString(`{"to":"","body":"hi"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/send", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	var out struct {
		OK           bool   `json:"ok"`
		ErrorCode    string `json:"error_code"`
		ErrorMessage string `json:"error_message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.ErrorCode != "invalid_destination" {
		t.Errorf("error_code = %q", out.ErrorCode)
	}
}

func TestSendHandler_RejectsMismatchedIdempotencyKeys(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("DingTalk must not be called for conflicting idempotency keys")
	}))
	defer server.Close()

	client := dingtalk.NewClientWithHTTP("k", "s", "1", &http.Client{Transport: &redirectTransport{base: server}})
	app := fiber.New()
	store := idempotency.NewStore(300)
	log := logger.New(logger.Config{Level: logger.ErrorLevel})
	app.Post("/v1/send", func(c *fiber.Ctx) error { return SendHandler(c, client, store, log) })

	body := bytes.NewBufferString(`{"to":"userid123","body":"hello","idempotency_key":"body-key"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/send", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "header-key")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("status = %d, want 409", resp.StatusCode)
	}
	var out struct {
		ErrorCode string `json:"error_code"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.ErrorCode != "idempotency_conflict" {
		t.Fatalf("error_code = %q", out.ErrorCode)
	}
}

func TestSendHandler_RejectsOversizedIdempotencyKey(t *testing.T) {
	client := dingtalk.NewClientWithHTTP("k", "s", "1", &http.Client{})
	app := fiber.New()
	store := idempotency.NewStore(300)
	log := logger.New(logger.Config{Level: logger.ErrorLevel})
	app.Post("/v1/send", func(c *fiber.Ctx) error { return SendHandler(c, client, store, log) })

	body := bytes.NewBufferString(`{"to":"userid123","body":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/send", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", strings.Repeat("x", maxIdempotencyKeyLength+1))
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestSendHandler_CachesSuccessfulIdempotentResponse(t *testing.T) {
	var sendCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gettoken":
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "token", "expires_in": 7200})
		case "/topapi/message/corpconversation/asyncsend_v2":
			sendCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "task_id": 777})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := dingtalk.NewClientWithHTTP("k", "s", "1", &http.Client{Transport: &redirectTransport{base: server}})
	app := fiber.New()
	store := idempotency.NewStore(300)
	log := logger.New(logger.Config{Level: logger.ErrorLevel})
	app.Post("/v1/send", func(c *fiber.Ctx) error { return SendHandler(c, client, store, log) })

	for i := 0; i < 2; i++ {
		body := bytes.NewBufferString(`{"to":"userid123","body":"hello","idempotency_key":"same-key"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/send", body)
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var out struct {
			MessageID string `json:"message_id"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&out)
		_ = resp.Body.Close()
		if out.MessageID != "777" {
			t.Fatalf("message_id = %q", out.MessageID)
		}
	}
	if sendCalls.Load() != 1 {
		t.Fatalf("send calls = %d, want 1", sendCalls.Load())
	}
}

func TestSendHandler_RejectsIdempotencyKeyReuseWithDifferentRequest(t *testing.T) {
	var sendCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gettoken":
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "token", "expires_in": 7200})
		case "/topapi/message/corpconversation/asyncsend_v2":
			sendCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "task_id": 777})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := dingtalk.NewClientWithHTTP("k", "s", "1", &http.Client{Transport: &redirectTransport{base: server}})
	app := fiber.New()
	store := idempotency.NewStore(300)
	log := logger.New(logger.Config{Level: logger.ErrorLevel})
	app.Post("/v1/send", func(c *fiber.Ctx) error { return SendHandler(c, client, store, log) })

	for i, message := range []string{"first", "second"} {
		body := bytes.NewBufferString(`{"to":"userid123","body":"` + message + `","idempotency_key":"same-key"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/send", body)
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		want := http.StatusOK
		if i == 1 {
			want = http.StatusConflict
		}
		if resp.StatusCode != want {
			_ = resp.Body.Close()
			t.Fatalf("request %d status = %d, want %d", i, resp.StatusCode, want)
		}
		_ = resp.Body.Close()
	}
	if sendCalls.Load() != 1 {
		t.Fatalf("send calls = %d, want 1", sendCalls.Load())
	}
}

func TestSendHandler_DoesNotCacheUpstreamFailure(t *testing.T) {
	var sendCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gettoken":
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "token", "expires_in": 7200})
		case "/topapi/message/corpconversation/asyncsend_v2":
			if sendCalls.Add(1) == 1 {
				_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 500, "errmsg": "temporary failure"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "task_id": 888})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := dingtalk.NewClientWithHTTP("k", "s", "1", &http.Client{Transport: &redirectTransport{base: server}})
	app := fiber.New()
	store := idempotency.NewStore(300)
	log := logger.New(logger.Config{Level: logger.ErrorLevel})
	app.Post("/v1/send", func(c *fiber.Ctx) error { return SendHandler(c, client, store, log) })

	for i := 0; i < 2; i++ {
		body := bytes.NewBufferString(`{"to":"userid123","body":"hello","idempotency_key":"retry-key"}`)
		req := httptest.NewRequest(http.MethodPost, "/v1/send", body)
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("app.Test: %v", err)
		}
		want := http.StatusBadGateway
		if i == 1 {
			want = http.StatusOK
		}
		if resp.StatusCode != want {
			_ = resp.Body.Close()
			t.Fatalf("request %d status = %d, want %d", i, resp.StatusCode, want)
		}
		_ = resp.Body.Close()
	}
	if sendCalls.Load() != 2 {
		t.Fatalf("send calls = %d, want 2", sendCalls.Load())
	}
}

func TestRequestFingerprintIsStableAcrossMapOrder(t *testing.T) {
	first, err := requestFingerprint(provider.HTTPSendRequest{
		Channel: "dingtalk", To: "user", Params: map[string]string{"a": "1", "b": "2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := requestFingerprint(provider.HTTPSendRequest{
		Channel: "dingtalk", To: "user", Params: map[string]string{"b": "2", "a": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("fingerprints differ: %s != %s", first, second)
	}
}

func TestSendHandler_RejectsUnsupportedChannel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("DingTalk must not be called for an unsupported channel")
	}))
	defer server.Close()

	client := dingtalk.NewClientWithHTTP("k", "s", "1", &http.Client{Transport: &redirectTransport{base: server}})
	idemStore := idempotency.NewStore(300)
	log := logger.New(logger.Config{Level: logger.ErrorLevel})
	app := fiber.New()
	app.Post("/v1/send", func(c *fiber.Ctx) error { return SendHandler(c, client, idemStore, log) })

	body := bytes.NewBufferString(`{"channel":"email","to":"userid123","body":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/send", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	var out struct {
		ErrorCode string `json:"error_code"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.ErrorCode != "invalid_request" {
		t.Errorf("error_code = %q, want invalid_request", out.ErrorCode)
	}
}

func TestSendHandler_RejectsInvalidTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("DingTalk must not be called for an invalid timeout")
	}))
	defer server.Close()

	client := dingtalk.NewClientWithHTTP("k", "s", "1", &http.Client{Transport: &redirectTransport{base: server}})
	idemStore := idempotency.NewStore(300)
	log := logger.New(logger.Config{Level: logger.ErrorLevel})
	app := fiber.New()
	app.Post("/v1/send", func(c *fiber.Ctx) error { return SendHandler(c, client, idemStore, log) })

	body := bytes.NewBufferString(`{"channel":"dingtalk","to":"userid123","body":"hello","timeout_seconds":31}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/send", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
	var out struct {
		ErrorCode string `json:"error_code"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.ErrorCode != "invalid_request" {
		t.Errorf("error_code = %q, want invalid_request", out.ErrorCode)
	}
}

func TestSendHandler_MapsUpstreamFailureToBadGateway(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gettoken":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "tok", "expires_in": 7200})
		case "/topapi/message/corpconversation/asyncsend_v2":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 500, "errmsg": "upstream failure"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := dingtalk.NewClientWithHTTP("k", "s", "1", &http.Client{Transport: &redirectTransport{base: server}})
	idemStore := idempotency.NewStore(300)
	log := logger.New(logger.Config{Level: logger.ErrorLevel})
	app := fiber.New()
	app.Post("/v1/send", func(c *fiber.Ctx) error { return SendHandler(c, client, idemStore, log) })

	body := bytes.NewBufferString(`{"channel":"dingtalk","to":"userid123","body":"hello"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/send", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("status = %d, want 502", resp.StatusCode)
	}
	var out struct {
		ErrorCode string `json:"error_code"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.ErrorCode != "send_failed" {
		t.Errorf("error_code = %q, want send_failed", out.ErrorCode)
	}
}

func TestSendHandler_AppliesRequestTimeout(t *testing.T) {
	client := dingtalk.NewClientWithHTTP("k", "s", "1", &http.Client{Transport: waitForCancellationTransport{}})
	idemStore := idempotency.NewStore(300)
	log := logger.New(logger.Config{Level: logger.ErrorLevel})
	app := fiber.New()
	app.Post("/v1/send", func(c *fiber.Ctx) error { return SendHandler(c, client, idemStore, log) })

	body := bytes.NewBufferString(`{"channel":"dingtalk","to":"userid123","body":"hello","timeout_seconds":1}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/send", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req, 3000)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Errorf("status = %d, want 504", resp.StatusCode)
	}
	var out struct {
		ErrorCode string `json:"error_code"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if out.ErrorCode != "timeout" {
		t.Errorf("error_code = %q, want timeout", out.ErrorCode)
	}
}

func TestIsTimeoutError(t *testing.T) {
	if !isTimeoutError(context.DeadlineExceeded) {
		t.Fatal("context deadline must be classified as timeout")
	}
	if !isTimeoutError(context.Canceled) {
		t.Fatal("context cancellation must be classified as timeout")
	}
	if isTimeoutError(nil) {
		t.Fatal("nil must not be classified as timeout")
	}
}

func TestMobileLikeRegex(t *testing.T) {
	// 与 send.go 中“仅数字且长度 11 视为手机号”的判定一致
	tests := []struct {
		to   string
		want bool
	}{
		{"13800138000", true},
		{"13912345678", true},
		{"10000000000", true},
		{"userid123", false},
		{"1380013800", false},
		{"138001380001", false},
		{"", false},
		{"1380013800a", false},
	}
	for _, tt := range tests {
		got := mobileLike.MatchString(tt.to)
		if got != tt.want {
			t.Errorf("mobileLike.MatchString(%q) = %v, want %v", tt.to, got, tt.want)
		}
	}
}

func TestSendHandler_MobileLookupWhenModeMobile(t *testing.T) {
	originalLookupMode := config.LookupMode
	config.LookupMode = config.LookupModeMobile
	t.Cleanup(func() { config.LookupMode = originalLookupMode })

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/gettoken":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "access_token": "tok", "expires_in": 7200})
		case "/topapi/v2/user/getbymobile":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "result": map[string]any{"userid": "uid-from-mobile"}})
		case "/topapi/message/corpconversation/asyncsend_v2":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"errcode": 0, "task_id": 888})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := dingtalk.NewClientWithHTTP("k", "s", "1", &http.Client{Transport: &redirectTransport{base: server}})
	idemStore := idempotency.NewStore(300)
	log := logger.New(logger.Config{Level: logger.ErrorLevel})
	app := fiber.New()
	app.Post("/v1/send", func(c *fiber.Ctx) error { return SendHandler(c, client, idemStore, log) })

	body := bytes.NewBufferString(`{"to":"13800138000","body":"code"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/send", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (with DINGTALK_LOOKUP_MODE=mobile)", resp.StatusCode)
	}
	var out struct {
		OK        bool   `json:"ok"`
		MessageID string `json:"message_id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	if !out.OK || out.MessageID != "888" {
		t.Errorf("ok=%v message_id=%q", out.OK, out.MessageID)
	}
}
