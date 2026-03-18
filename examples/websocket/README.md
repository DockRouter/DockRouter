# WebSocket Proxying Example

This example demonstrates DockRouter's WebSocket proxying capabilities with sticky sessions support.

## Features Demonstrated

- **WebSocket passthrough** - Real-time bidirectional communication
- **Sticky sessions** - Session affinity using IP hash load balancing
- **Multiple backends** - WebSocket connections distributed across instances

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌─────────────────┐
│   Client    │────▶│  DockRouter │────▶│  WS Backend 1   │
│  (Browser)  │     │   (:80/:443)│     │  (ws-backend-1) │
└─────────────┘     └─────────────┘     └─────────────────┘
                             │
                             └──────────▶│  WS Backend 2   │
                                          │  (ws-backend-2) │
                                          └─────────────────┘
```

## Running the Example

```bash
cd examples/websocket
docker-compose up -d
```

## Testing WebSocket

Connect to `ws://localhost/ws` using a WebSocket client:

```javascript
// Browser console
const ws = new WebSocket('ws://localhost/ws');
ws.onmessage = (e) => console.log('Received:', e.data);
ws.send('Hello from client!');
```

Or using `wscat`:

```bash
npm install -g wscat
wscat -c ws://localhost/ws
```

## Configuration Details

### DockRouter Labels

```yaml
labels:
  - "dr.enabled=true"
  - "dr.host=localhost"
  - "dr.port=8080"
  - "dr.loadbalancer=iphash"  # Sticky sessions
```

### Sticky Sessions

The `iphash` load balancing strategy ensures:
- Same client IP always connects to the same backend
- WebSocket connections maintain session state
- Graceful handling of connection upgrades

## How It Works

1. **Connection Upgrade**: DockRouter detects WebSocket upgrade request
2. **Backend Selection**: IP hash selects appropriate backend
3. **Hijacking**: Connection is hijacked for bidirectional data flow
4. **Data Copy**: Goroutines handle client↔backend data copying

## Cleanup

```bash
docker-compose down -v
```
