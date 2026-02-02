# herald-dingtalk Troubleshooting Guide

This guide helps you diagnose and resolve common issues with herald-dingtalk.

## Table of Contents

- [DingTalk Message Not Received](#dingtalk-message-not-received)
- [503 provider_down](#503-provider_down)
- [401 Unauthorized](#401-unauthorized)
- [invalid_destination](#invalid_destination)
- [Idempotency and Logs](#idempotency-and-logs)

## DingTalk Message Not Received

### Symptoms

- Herald creates a challenge with channel `dingtalk` and gets a successful response from herald-dingtalk, but the user does not receive a DingTalk message.

### Diagnostic Steps

1. **Check herald-dingtalk logs**  
   Look for `send_failed` or DingTalk API errors:
   ```bash
   # If running in Docker
   docker logs herald-dingtalk 2>&1 | grep -E "send_failed|send ok|errcode"
   ```
   - `send ok` with `message_id`: herald-dingtalk successfully called DingTalk; delivery issues may be on DingTalk or user side.
   - `send_failed` with errmsg: note the DingTalk `errcode` and `errmsg` for the next steps.

2. **Verify DingTalk configuration**  
   - Confirm `DINGTALK_APP_KEY`, `DINGTALK_APP_SECRET`, and `DINGTALK_AGENT_ID` are set and match the DingTalk open platform.
   - In the DingTalk console, check that the app has “work notification” permission and that the app is published/enabled.

3. **Check visibility and userid**  
   - The `to` field must be the DingTalk **userid** (not mobile or email). If Herald passes the wrong identifier (e.g. phone number), DingTalk may reject or not deliver.
   - Ensure the target user is within the app’s visible range (visible to the whole org or to selected depts/users).

4. **Verify DingTalk API limits**  
   - Check whether the app has hit rate or quota limits in the DingTalk open platform.

### Solutions

- **Wrong credentials**: Update `DINGTALK_APP_KEY`, `DINGTALK_APP_SECRET`, `DINGTALK_AGENT_ID` and restart herald-dingtalk.
- **Wrong or invalid userid**: Ensure Herald (or Warden) resolves the user to a valid DingTalk userid and passes it as `destination` for channel `dingtalk`.
- **Permission or visibility**: Adjust app permissions and visible range in the DingTalk console.

---

## 503 provider_down

### Symptoms

- `POST /v1/send` returns HTTP 503 with body: `"ok": false, "error_code": "provider_down", "error_message": "dingtalk not configured"`.

### Cause

At startup, herald-dingtalk checks that all three DingTalk settings are non-empty. If any of `DINGTALK_APP_KEY`, `DINGTALK_APP_SECRET`, or `DINGTALK_AGENT_ID` is missing, the DingTalk client is not initialized and every send returns 503.

### Solutions

1. Set all three environment variables and restart the process (or container).
2. Confirm they are actually present in the runtime (e.g. no typo in env names, and in Docker/Kubernetes they are passed correctly).
3. Check logs at startup: if credentials are missing, herald-dingtalk logs a warning that `/v1/send` will return 503.

---

## 401 Unauthorized

### Symptoms

- `POST /v1/send` returns HTTP 401 with `error_code: "unauthorized"`, `error_message: "invalid or missing API key"`.

### Cause

herald-dingtalk has `API_KEY` set, but the request either does not send `X-API-Key` or sends a value that does not match.

### Solutions

1. **If you intend to use API Key**  
   - Set `API_KEY` on herald-dingtalk.  
   - Set `HERALD_DINGTALK_API_KEY` on Herald to the same value so Herald sends it in `X-API-Key`.  
   - Ensure no proxy or gateway strips the `X-API-Key` header.

2. **If you do not want API Key auth**  
   - Leave `API_KEY` unset on herald-dingtalk (and do not set `HERALD_DINGTALK_API_KEY` on Herald).

---

## invalid_destination

### Symptoms

- `POST /v1/send` returns HTTP 400 with `error_code: "invalid_destination"`, `error_message: "to is required"`.

### Cause

The request body has an empty or missing `to` field. For herald-dingtalk, `to` must be the DingTalk userid.

### Solutions

1. Ensure Herald sends a non-empty `to` (destination) when calling herald-dingtalk. For channel `dingtalk`, Herald should pass the DingTalk userid as the destination (from Warden or your user store).
2. Check that the mapping from “user identifier” to “DingTalk userid” is correct and never yields an empty string.

---

## Idempotency and Logs

### Idempotent hit (cached response)

When Herald (or any client) sends the same `Idempotency-Key` (or body `idempotency_key`) within the configured TTL, herald-dingtalk returns the cached response without calling DingTalk again. This is expected.

### Log level

- **info**: You see `send ok` and `send_failed` (and 503/401 as above).
- **debug**: You also see `send idempotent hit` for cached responses. Set `LOG_LEVEL=debug` to verify that repeated requests with the same idempotency key are being cached.

### TTL

Idempotency cache TTL is controlled by `IDEMPOTENCY_TTL_SECONDS` (default 300). After TTL, the same key is treated as a new request and may trigger a new DingTalk send.
