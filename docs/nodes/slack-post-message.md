# Slack message action

`slack.postMessage@1` sends `{ "text": "Fixture message" }` to the configured channel ID through `https://slack.com/api/chat.postMessage`. Configuration requires `channel` and `credentialRef`. Use a Slack bot token with `chat:write` and invite the bot to the target channel. The credential destination is `slack.com`.

The output contains only `channel` and the string message timestamp `ts`. HTTP 200 with `ok: false` becomes `ProviderRejected`; raw provider errors are not persisted. Invalid success bodies become `InvalidOutput`.

Message creation is an unsafe write. An uncertain response needs operator reconciliation; Runnelix does not automatically resend it. This action does not claim exactly-once provider delivery. Tests use fixtures only; no real Slack message is sent.
