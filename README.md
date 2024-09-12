# nats-http-gateway

This is an open-source alternative to the Synadia Cloud's HTTP Gateway, offering a first-class HTTP interface for NATS. It provides HTTP access to NATS capabilities such as key-value stores, object storage, messaging, and services, making NATS even more accessible for developers familiar with HTTP.

**Note:** This project is a work in progress and currently does not provide any authentication mechanisms. Please use at your own risk.

For reference and detailed documentation, check out the [Synadia Cloud HTTP Gateway documentation](https://docs.synadia.com/cloud/resources/http-gateway).

### Usage
You can import the `natshttp` package and use it in your project, making sure that the `Handler` struct implements the NATS connection management.

Example:

```go
import "github.com/gleez/nats-http-gateway"

// Initialize and use the handlers
h := &natshttp.Handler{nc: YourNatsConnection}
http.HandleFunc("/nats", h.NatsHandler)
```