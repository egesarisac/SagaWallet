module github.com/egesarisac/SagaWallet/tools/eventreplay

go 1.25.0

require (
	github.com/egesarisac/SagaWallet/pkg v0.0.0
	github.com/google/uuid v1.6.0
)

require (
	github.com/klauspost/compress v1.17.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.19 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
	github.com/rs/zerolog v1.31.0 // indirect
	github.com/segmentio/kafka-go v0.4.47 // indirect
	golang.org/x/sys v0.41.0 // indirect
)

replace github.com/egesarisac/SagaWallet/pkg => ../../pkg
