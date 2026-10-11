package grpcserver

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/e-l-l-a-r/monitoring/internal/auditor"
	"github.com/e-l-l-a-r/monitoring/internal/crypto"
	"github.com/e-l-l-a-r/monitoring/internal/handler"
	"github.com/e-l-l-a-r/monitoring/internal/model"
	pb "github.com/e-l-l-a-r/monitoring/internal/proto"
	"github.com/e-l-l-a-r/monitoring/internal/repository"
)

// startServer запускает gRPC-сервер в памяти и возвращает клиента и хранилище.
func startServer(t *testing.T, cidr, key string) (pb.MetricsClient, handler.Storage) {
	t.Helper()

	trusted, err := handler.NewTrustedSubnet(cidr)
	require.NoError(t, err)
	storage := repository.NewMemStorage(300, t.TempDir()+"/metrics.json")
	audit := auditor.NewAuditor()
	t.Cleanup(audit.Close)

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.ChainUnaryInterceptor(
		TrustedSubnetInterceptor(trusted),
		SignInterceptor(crypto.NewSigner(key)),
	))
	pb.RegisterMetricsServer(srv, NewMetricsServer(storage, audit))
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return pb.NewMetricsClient(conn), storage
}

func batchRequest() *pb.UpdateMetricsRequest {
	return &pb.UpdateMetricsRequest{Metrics: []*pb.Metric{
		{Id: "PollCount", Type: pb.Metric_COUNTER, Delta: 5},
		{Id: "Alloc", Type: pb.Metric_GAUGE, Value: 1.5},
	}}
}

func TestUpdateMetrics(t *testing.T) {
	client, storage := startServer(t, "", "")

	_, err := client.UpdateMetrics(context.Background(), batchRequest())
	require.NoError(t, err)

	vals := storage.GetValues(context.Background())
	require.Len(t, vals, 2)
	cnt, err := storage.GetValue(context.Background(), "PollCount", model.Counter)
	require.NoError(t, err)
	assert.InDelta(t, 5.0, cnt, 1e-9)
	gauge, err := storage.GetValue(context.Background(), "Alloc", model.Gauge)
	require.NoError(t, err)
	assert.InDelta(t, 1.5, gauge, 1e-9)
}

func TestUpdateMetricsInvalidArgument(t *testing.T) {
	client, _ := startServer(t, "", "")

	_, err := client.UpdateMetrics(context.Background(), &pb.UpdateMetricsRequest{
		Metrics: []*pb.Metric{{Id: "", Type: pb.Metric_GAUGE}},
	})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestTrustedSubnetInterceptor(t *testing.T) {
	tests := []struct {
		name string
		cidr string
		ip   string
		want codes.Code
	}{
		{name: "подсеть не задана", cidr: "", ip: "", want: codes.OK},
		{name: "IP из доверенной подсети", cidr: "192.168.1.0/24", ip: "192.168.1.10", want: codes.OK},
		{name: "IP вне подсети", cidr: "192.168.1.0/24", ip: "10.0.0.1", want: codes.PermissionDenied},
		{name: "нет x-real-ip", cidr: "192.168.1.0/24", ip: "", want: codes.PermissionDenied},
		{name: "некорректный IP", cidr: "192.168.1.0/24", ip: "bad", want: codes.PermissionDenied},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := startServer(t, tt.cidr, "")
			ctx := context.Background()
			if tt.ip != "" {
				ctx = metadata.AppendToOutgoingContext(ctx, RealIPKey, tt.ip)
			}
			_, err := client.UpdateMetrics(ctx, batchRequest())
			assert.Equal(t, tt.want, status.Code(err))
		})
	}
}

func TestSignInterceptor(t *testing.T) {
	const key = "secret"
	req := batchRequest()
	goodSign, err := SignMessage(crypto.NewSigner(key), req)
	require.NoError(t, err)
	require.NotEmpty(t, goodSign)

	tests := []struct {
		name      string
		serverKey string
		sign      string
		want      codes.Code
	}{
		{name: "верная подпись", serverKey: key, sign: goodSign, want: codes.OK},
		{name: "неверная подпись", serverKey: key, sign: "bad", want: codes.InvalidArgument},
		{name: "без подписи", serverKey: key, sign: "", want: codes.OK},
		{name: "ключ на сервере не задан", serverKey: "", sign: "bad", want: codes.OK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := startServer(t, "", tt.serverKey)
			ctx := context.Background()
			if tt.sign != "" {
				ctx = metadata.AppendToOutgoingContext(ctx, SignKey, tt.sign)
			}
			_, err := client.UpdateMetrics(ctx, req)
			assert.Equal(t, tt.want, status.Code(err))
		})
	}
}

func TestProtoConversion(t *testing.T) {
	src := []model.Metrics{
		model.NewCounterMetrics("c", 7),
		model.NewGaugeMetrics("g", 2.5),
		{ID: "unknown", MType: "other"},
	}
	msgs := ToProto(src)
	require.Len(t, msgs, 2)

	back, err := FromProto(msgs)
	require.NoError(t, err)
	assert.Equal(t, src[:2], back)
}
