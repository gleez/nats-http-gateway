# nats-http-gateway

This is an open-source alternative to the Synadia Cloud's HTTP Gateway, offering a first-class HTTP interface for [NATS](http://nats.io/). It provides HTTP access to [NATS](http://nats.io/) capabilities such as key-value stores, object storage, messaging, and services, making NATS even more accessible for developers familiar with HTTP.

**Note:** This project is a work in progress and currently does not provide any authentication mechanisms. Please use at your own risk.

For reference and detailed documentation, check out the [Synadia Cloud HTTP Gateway documentation](https://docs.synadia.com/cloud/resources/http-gateway).

### Usage
You can import the `natshttp` package and use it in your project, making sure that the `Handler` struct implements the [NATS connection management](https://github.com/nats-io/nats.go#advanced-usage).

Example:

```go
import "github.com/gleez/nats-http-gateway"

// Initialize and use the handlers
h := natshttp.New(nc)
http.HandleFunc("/api/v1/nats/subjects/", h.NatsHandler)
```

### Similar Projects
If you’re looking for other projects in the NATS HTTP Gateway space, be sure to check out:

[hats](https://github.com/RussellLuo/hats): Another NATS HTTP gateway implementation, providing similar functionality for HTTP to NATS interaction. Kudos to RussellLuo for the great work!