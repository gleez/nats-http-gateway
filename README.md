# nats-http-gateway

An open‑source alternative to the Synadia Cloud HTTP Gateway, exposing NATS functionality over a clean HTTP interface. The gateway currently supports two main API groups:

* **Core messaging** – publish, request/reply and server‑sent‑event subscription via the `/nats/subjects/` path.
* **Key‑Value store API** – create, read, update and delete KV entries using the `/kv/{bucket}/{key}` path, matching the OpenAPI specification.

Authentication is handled **via command‑line flags** when running the server binary:

```bash
nats-http-gateway \
    -s nats://demo.nats.io:4222          # NATS server URL (default: nats://127.0.0.1:4222)
    -creds path/to/user.creds            # optional user credentials file
    -nkey  path/to/seed.nk               # optional NKey seed file
    -token <jwt-token>                   # optional JWT token
```

The server reads the flags at startup and builds the appropriate NATS connection options.

> **Note:** No built‑in role‑based access control is provided; use the NATS server's security features (credentials, NKey or token) to protect the gateway.

For full specification details, see the [Synadia Cloud HTTP Gateway documentation](https://docs.synadia.com/cloud/resources/http-gateway).

### Usage example (in Go)
```go
import (
    "net/http"
    "github.com/gleez/nats-http-gateway"
    "github.com/nats-io/nats.go"
)

func main() {
    // Connect to NATS (flags can also be used when running the binary).
    nc, _ := nats.Connect(nats.DefaultURL)
    h := natshttp.New(nc)
    // Register the handler – the gateway routes internally based on the path.
    http.HandleFunc("/api/v1/", h.NatsHandler)
    http.ListenAndServe(":8080", nil)
}
```

### Similar projects
* [hats](https://github.com/RussellLuo/hats) – another NATS HTTP gateway implementation.
