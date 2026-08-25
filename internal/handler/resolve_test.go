package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/soulteary/herald-dingtalk/internal/dingtalk"
	"github.com/soulteary/logger-kit/v2"
)

func TestResolveHandler_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1.0/oauth2/userAccessToken":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "t", "expireIn": 7200})
		case "/v1.0/contact/users/me":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"userId": "resolved-user-1", "unionId": "u1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := dingtalk.NewClientWithHTTP("k", "s", "1", &http.Client{Transport: &redirectTransport{base: server}})
	log := logger.New(logger.Config{Level: logger.ErrorLevel})
	app := fiber.New()
	app.Post("/v1/resolve", func(c fiber.Ctx) error { return ResolveHandler(c, client, log) })

	body := bytes.NewBufferString(`{"auth_code":"code123"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/resolve", body)
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
		OK     bool   `json:"ok"`
		UserID string `json:"userid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !out.OK || out.UserID != "resolved-user-1" {
		t.Errorf("ok=%v userid=%q", out.OK, out.UserID)
	}
}

func TestResolveHandler_EmptyAuthCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	client := dingtalk.NewClientWithHTTP("k", "s", "1", &http.Client{Transport: &redirectTransport{base: server}})
	log := logger.New(logger.Config{Level: logger.ErrorLevel})
	app := fiber.New()
	app.Post("/v1/resolve", func(c fiber.Ctx) error { return ResolveHandler(c, client, log) })

	body := bytes.NewBufferString(`{"auth_code":""}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/resolve", body)
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
	if out.ErrorCode != "invalid_request" {
		t.Errorf("error_code = %q", out.ErrorCode)
	}
}

func TestResolveHandler_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	client := dingtalk.NewClientWithHTTP("k", "s", "1", &http.Client{Transport: &redirectTransport{base: server}})
	log := logger.New(logger.Config{Level: logger.ErrorLevel})
	app := fiber.New()
	app.Post("/v1/resolve", func(c fiber.Ctx) error { return ResolveHandler(c, client, log) })

	body := bytes.NewBufferString(`not json`)
	req := httptest.NewRequest(http.MethodPost, "/v1/resolve", body)
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", resp.StatusCode)
	}
}

func TestResolveHandler_RejectsUnsupportedMediaType(t *testing.T) {
	client := dingtalk.NewClientWithHTTP("k", "s", "1", &http.Client{})
	log := logger.New(logger.Config{Level: logger.ErrorLevel})
	app := fiber.New()
	app.Post("/v1/resolve", func(c fiber.Ctx) error { return ResolveHandler(c, client, log) })
	req := httptest.NewRequest(http.MethodPost, "/v1/resolve", strings.NewReader(`{"auth_code":"code"}`))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", resp.StatusCode)
	}
}

func TestResolveHandler_RejectsUnsafeAuthCode(t *testing.T) {
	client := dingtalk.NewClientWithHTTP("k", "s", "1", &http.Client{})
	for _, code := range []string{" code", strings.Repeat("x", maxAuthCodeLength+1)} {
		app := fiber.New()
		app.Post("/v1/resolve", func(c fiber.Ctx) error {
			return ResolveHandler(c, client, logger.New(logger.Config{Level: logger.ErrorLevel}))
		})
		body, _ := json.Marshal(ResolveRequest{AuthCode: code})
		req := httptest.NewRequest(http.MethodPost, "/v1/resolve", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	}
}

func TestResolveHandler_ProviderDown(t *testing.T) {
	app := fiber.New()
	app.Post("/v1/resolve", func(c fiber.Ctx) error {
		return ResolveHandler(c, nil, logger.New(logger.Config{Level: logger.Disabled}))
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/resolve", bytes.NewBufferString(`{"auth_code":"code"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", resp.StatusCode)
	}
}

func TestResolveHandler_MapsOAuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "expired code"})
	}))
	defer server.Close()
	client := dingtalk.NewClientWithHTTP("k", "s", "1", &http.Client{Transport: &redirectTransport{base: server}})
	app := fiber.New()
	app.Post("/v1/resolve", func(c fiber.Ctx) error {
		return ResolveHandler(c, client, logger.New(logger.Config{Level: logger.Disabled}))
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/resolve", bytes.NewBufferString(`{"auth_code":"code"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}
