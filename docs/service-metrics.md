# Service metrics

`GET /metrics` exposes process-level Prometheus text metrics. Request latency is measured from middleware entry to response completion and labelled by method, bounded route template, and status. Webhook lag is measured from the event's receipt timestamp to completed synchronous persistence; it is labelled only by provider. Websocket queue depth is the number of pending invalidations in each subscriber channel. No tenant content, secrets, delivery IDs, or raw URLs are included.
