module github.com/WolffunGame/wolfsocket

go 1.18

require (
	github.com/RussellLuo/timingwheel v0.0.0-20220218152713-54845bda3108
	github.com/gobwas/ws v1.1.0
	github.com/gorilla/websocket v1.5.0
	github.com/iris-contrib/go.uuid v2.0.0+incompatible
	github.com/nats-io/nats.go v1.16.0
	github.com/redis/go-redis/v9 v9.0.2
	google.golang.org/protobuf v1.28.0
)

require (
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/nats-io/nats-server/v2 v2.8.4 // indirect
	github.com/nats-io/nkeys v0.3.0 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	golang.org/x/crypto v0.0.0-20220315160706-3147a52a75dd // indirect
	golang.org/x/sys v0.0.0-20220111092808-5a964db01320 // indirect
	gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c // indirect
)

replace github.com/kataras/neffos => github.com/WolffunGame/wolfsocket v0.1.2
