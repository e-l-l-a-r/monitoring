// Package grpcserver реализует gRPC-сервис Metrics для приёма метрик от агентов
// и серверные интерцепторы: проверку доверенной подсети и подписи запроса.
package grpcserver

import (
	"context"
	"fmt"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"github.com/e-l-l-a-r/monitoring/internal/auditor"
	"github.com/e-l-l-a-r/monitoring/internal/crypto"
	"github.com/e-l-l-a-r/monitoring/internal/handler"
	"github.com/e-l-l-a-r/monitoring/internal/logger"
	"github.com/e-l-l-a-r/monitoring/internal/model"
	pb "github.com/e-l-l-a-r/monitoring/internal/proto"
)

const (
	// RealIPKey - ключ метаданных, в котором агент передаёт IP-адрес своего хоста.
	RealIPKey = "x-real-ip"
	// SignKey - ключ метаданных с HMAC-SHA256 подписью сериализованного запроса.
	SignKey = "hashsha256"
)

// MetricsServer реализует gRPC-сервис Metrics поверх хранилища метрик.
type MetricsServer struct {
	pb.UnimplementedMetricsServer
	storage handler.Storage
	audit   auditor.Publisher
}

// NewMetricsServer создаёт сервис Metrics. audit может быть nil - тогда аудит
// не ведётся.
func NewMetricsServer(storage handler.Storage, audit auditor.Publisher) *MetricsServer {
	return &MetricsServer{storage: storage, audit: audit}
}

// UpdateMetrics сохраняет пакет метрик из запроса и уведомляет аудит.
func (s *MetricsServer) UpdateMetrics(ctx context.Context, req *pb.UpdateMetricsRequest) (*pb.UpdateMetricsResponse, error) {
	metrics, err := FromProto(req.GetMetrics())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	err = logger.ExecuteWithRetryNoResult(func(args ...interface{}) error {
		return s.storage.AddBatchMetricsData(ctx, metrics)
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "не удалось сохранить метрики: %v", err)
	}

	if s.audit != nil {
		names := make([]string, len(metrics))
		for i, m := range metrics {
			names[i] = m.ID
		}
		data := auditor.NewAuditData(names, clientIP(ctx))
		if e := s.audit.Notify(&data); e != nil {
			logger.Warn("Audit error: ", e.Error())
			return nil, status.Error(codes.Internal, "ошибка аудита")
		}
	}

	if e := s.storage.SyncIfNeed(ctx); e != nil {
		logger.Warn("sync error: ", e.Error())
	}

	return &pb.UpdateMetricsResponse{}, nil
}

// FromProto переводит метрики протокола в модель хранилища. Метрика без имени
// или с неизвестным типом возвращает ошибку.
func FromProto(src []*pb.Metric) ([]model.Metrics, error) {
	result := make([]model.Metrics, 0, len(src))
	for _, m := range src {
		if m.GetId() == "" {
			return nil, fmt.Errorf("пустое имя метрики")
		}
		switch m.GetType() {
		case pb.Metric_COUNTER:
			result = append(result, model.NewCounterMetrics(m.GetId(), m.GetDelta()))
		case pb.Metric_GAUGE:
			result = append(result, model.NewGaugeMetrics(m.GetId(), m.GetValue()))
		default:
			return nil, fmt.Errorf("неизвестный тип метрики %q: %v", m.GetId(), m.GetType())
		}
	}
	return result, nil
}

// ToProto переводит метрики модели в сообщения протокола. Метрики неизвестного
// типа пропускаются.
func ToProto(src []model.Metrics) []*pb.Metric {
	result := make([]*pb.Metric, 0, len(src))
	for _, m := range src {
		switch m.MType {
		case model.Counter:
			var delta int64
			if m.Delta != nil {
				delta = *m.Delta
			}
			result = append(result, &pb.Metric{Id: m.ID, Type: pb.Metric_COUNTER, Delta: delta})
		case model.Gauge:
			var value float64
			if m.Value != nil {
				value = *m.Value
			}
			result = append(result, &pb.Metric{Id: m.ID, Type: pb.Metric_GAUGE, Value: value})
		}
	}
	return result
}

// SignMessage вычисляет HMAC-SHA256 подпись детерминированно сериализованного
// сообщения msg. Если подпись выключена, возвращает пустую строку.
func SignMessage(signer *crypto.Signer, msg proto.Message) (string, error) {
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(msg)
	if err != nil {
		return "", fmt.Errorf("не удалось сериализовать сообщение: %w", err)
	}
	return signer.SignData(data), nil
}

// metadataValue возвращает первое значение ключа key из входящих метаданных.
func metadataValue(ctx context.Context, key string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	values := md.Get(key)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

// clientIP возвращает IP-адрес агента из метаданных x-real-ip, а если его нет -
// адрес удалённой стороны соединения.
func clientIP(ctx context.Context) string {
	if ip := metadataValue(ctx, RealIPKey); ip != "" {
		return ip
	}
	if p, ok := peer.FromContext(ctx); ok && p.Addr != nil {
		host, _, err := net.SplitHostPort(p.Addr.String())
		if err != nil {
			return p.Addr.String()
		}
		return host
	}
	return ""
}

// TrustedSubnetInterceptor возвращает интерцептор, отклоняющий с кодом
// codes.PermissionDenied запросы, у которых IP-адрес из метаданных x-real-ip
// отсутствует, некорректен или не входит в доверенную подсеть. Выключенный
// фильтр пропускает все запросы.
func TrustedSubnetInterceptor(trusted *handler.TrustedSubnet) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		// Выключенный фильтр Contains считает доверенным любой адрес.
		ip := metadataValue(ctx, RealIPKey)
		if !trusted.Contains(ip) {
			return nil, status.Errorf(codes.PermissionDenied, "IP-адрес %q не входит в доверенную подсеть", ip)
		}
		return next(ctx, req)
	}
}

// SignInterceptor возвращает интерцептор, проверяющий подпись запроса из
// метаданных hashsha256. Как и в HTTP, запрос без подписи пропускается, а
// запрос с неверной подписью отклоняется с кодом codes.InvalidArgument.
// Выключенный signer пропускает все запросы.
func SignInterceptor(signer *crypto.Signer) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		sign := metadataValue(ctx, SignKey)
		if sign == "" {
			return next(ctx, req)
		}
		msg, ok := req.(proto.Message)
		if !ok {
			return nil, status.Error(codes.InvalidArgument, "запрос не является protobuf-сообщением")
		}
		expected, err := SignMessage(signer, msg)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		// Пустая подпись означает, что ключ на сервере не задан.
		if expected == "" {
			return next(ctx, req)
		}
		if expected != sign {
			logger.Warn("Bad sign: ", expected, " != ", sign)
			return nil, status.Error(codes.InvalidArgument, "bad sign")
		}
		return next(ctx, req)
	}
}
