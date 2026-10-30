# Browser WebSocket authentication

Browser `WebSocket` constructors cannot set an `Authorization` header. The worker-log WebSocket endpoint therefore accepts the existing selected-organization JWT in the `access_token` query parameter, but only on a genuine WebSocket upgrade request.

```js
const socket = new WebSocket(
  `${apiOrigin}/api/v1/agent-runs/${runID}/logs?access_token=${encodeURIComponent(selectedOrganizationJWT)}`,
);
```

The request must have `Connection: Upgrade` and `Upgrade: websocket`, as sent by a browser WebSocket handshake. The server validates `access_token` with the same JWT session validation as `Authorization: Bearer`: signature, algorithm, required expiry, token expiry, account identity, and selected `organization_id`. Invalid, expired, malformed, or organization-less sessions are rejected. REST requests cannot use this query parameter.

Existing Bearer authentication remains supported for non-browser clients. The token is not returned in an HTTP response and is not negotiated as a WebSocket subprotocol. Applications should use TLS (`wss://`) so the URL credential is encrypted in transit, and should avoid logging full handshake URLs.

This transport does not change authorization. Downstream tenant and role checks still run in their controllers and middleware, and cross-origin policy is unchanged.
