---
service_id: legacy-mailer
name: Legacy Mailer
capability: notification
status: deprecated
tier: 9
endpoint: http://legacy-mailer.internal:25
owner_team: platform-messaging
selectors: ["email", "mail", "smtp"]
superseded_by: notify-svc
policy_tags:
  region: us
  pii: unmanaged
---

## API usage (deprecated — do not integrate)

The Legacy Mailer is an SMTP relay retained only for services not yet migrated.
It performs no PII redaction and has no retry semantics. It is **deprecated**
and superseded by `notify-svc`; new integrations MUST use the Notification
Service instead. This record exists so discovery can name the successor.
