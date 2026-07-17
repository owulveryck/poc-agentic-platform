---
service_id: notify-svc
name: Notification Service
capability: notification
status: recommended
tier: 1
endpoint: http://localhost:9110
owner_team: platform-messaging
selectors: ["notification", "notify", "email", "e-mail", "sms", "message", "alert"]
supersedes: ["legacy-mailer"]
policy_tags:
  region: eu
  pii: handled
---

## API usage

Send a message through the platform Notification Service. All channels (email,
SMS, push) go through the same endpoint; the service handles provider fan-out,
retries, and PII redaction — do not call an email/SMS provider directly.

```
POST http://localhost:9110/v1/messages
Content-Type: application/json

{
  "channel": "email",           // "email" | "sms" | "push"
  "to": "user@example.com",
  "template": "welcome",
  "data": { "name": "Ada" }
}
```

Response `202 Accepted`:

```json
{ "id": "msg_abc123", "status": "queued" }
```

Idempotency: pass a unique `Idempotency-Key` header to make retries safe.
Auth in production is a platform service token in `Authorization: Bearer …`;
the local mock accepts any value.
