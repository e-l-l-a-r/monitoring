// Package proto содержит код, сгенерированный protoc по описанию протокола
// api/proto/metrics.proto: сообщения метрик и gRPC-сервис Metrics.
package proto

//go:generate sh -c "protoc --proto_path=../../api/proto --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative metrics.proto"
