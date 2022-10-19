// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/ingester/client/client.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package client

import (
	"flag"
	"fmt"
	"io"
	"sync"

	"github.com/go-kit/log"
	"github.com/grafana/dskit/grpcclient"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"

	"github.com/grafana/mimir/pkg/mimirpb"
)

//lint:ignore faillint It's non-trivial to remove this global variable.
var ingesterClientRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "cortex_ingester_client_request_duration_seconds",
	Help:    "Time spent doing Ingester requests.",
	Buckets: prometheus.ExponentialBuckets(0.001, 4, 8),
}, []string{"operation", "status_code"})

// HealthAndIngesterClient is the union of IngesterClient and grpc_health_v1.HealthClient.
type HealthAndIngesterClient interface {
	IngesterClient
	grpc_health_v1.HealthClient
	Close() error
}

type closableHealthAndIngesterClient struct {
	IngesterClient
	grpc_health_v1.HealthClient
	conn *grpc.ClientConn
}

// MakeIngesterClient makes a new IngesterClient
func MakeIngesterClient(addr string, cfg Config) (HealthAndIngesterClient, error) {
	dialOpts, err := cfg.GRPCClientConfig.DialOption(grpcclient.Instrument(ingesterClientRequestDuration))
	if err != nil {
		return nil, err
	}
	conn, err := grpc.Dial(addr, dialOpts...)
	if err != nil {
		return nil, err
	}
	return &closableHealthAndIngesterClient{
		IngesterClient: NewIngesterClient(conn),
		HealthClient:   grpc_health_v1.NewHealthClient(conn),
		conn:           conn,
	}, nil
}

func (c *closableHealthAndIngesterClient) Close() error {
	return c.conn.Close()
}

// Config is the configuration struct for the ingester client
type Config struct {
	GRPCClientConfig grpcclient.Config `yaml:"grpc_client_config" doc:"description=Configures the gRPC client used to communicate between distributors and ingesters."`
}

// RegisterFlags registers configuration settings used by the ingester client config.
func (cfg *Config) RegisterFlags(f *flag.FlagSet) {
	cfg.GRPCClientConfig.RegisterFlagsWithPrefix("ingester.client", f)
}

func (cfg *Config) Validate(log log.Logger) error {
	return cfg.GRPCClientConfig.Validate(log)
}

// Prealloc_Ingester_QueryStreamClient extends the Ingester_QueryStreamClient interface
// adding a poolable response receiver method.
type Prealloc_Ingester_QueryStreamClient interface {
	Ingester_QueryStreamClient
	RecvAt(*PreallocQueryStreamResponse) error
}

func (x *ingesterQueryStreamClient) RecvAt(m *PreallocQueryStreamResponse) error {
	return x.ClientStream.RecvMsg(m)
}

// PreallocQueryStreamResponse is a reusable response type for the QueryStream RPC.
type PreallocQueryStreamResponse struct {
	*QueryStreamResponse
}

func (m *PreallocQueryStreamResponse) Reset() {
	*m = PreallocQueryStreamResponse{&QueryStreamResponse{}}
}

func (m *PreallocQueryStreamResponse) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowIngester
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return fmt.Errorf("proto: QueryStreamResponse: wiretype end group for non-group")
		}
		if fieldNum <= 0 {
			return fmt.Errorf("proto: QueryStreamResponse: illegal tag %d (wire type %d)", fieldNum, wire)
		}
		switch fieldNum {
		case 1:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Chunkseries", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowIngester
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthIngester
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthIngester
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Chunkseries = append(m.Chunkseries, timeChunkSeriesFromPool())
			if err := m.Chunkseries[len(m.Chunkseries)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		case 2:
			if wireType != 2 {
				return fmt.Errorf("proto: wrong wireType = %d for field Timeseries", wireType)
			}
			var msglen int
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowIngester
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				msglen |= int(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			if msglen < 0 {
				return ErrInvalidLengthIngester
			}
			postIndex := iNdEx + msglen
			if postIndex < 0 {
				return ErrInvalidLengthIngester
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			m.Timeseries = append(m.Timeseries, mimirTimeSeriesFromPool())
			if err := m.Timeseries[len(m.Timeseries)-1].Unmarshal(dAtA[iNdEx:postIndex]); err != nil {
				return err
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipIngester(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if skippy < 0 {
				return ErrInvalidLengthIngester
			}
			if (iNdEx + skippy) < 0 {
				return ErrInvalidLengthIngester
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}

	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}

var (
	streamRespPool = sync.Pool{
		New: func() interface{} {
			return &PreallocQueryStreamResponse{&QueryStreamResponse{}}
		},
	}
	chunksPool        = sync.Pool{New: func() interface{} { return &[]Chunk{} }}
	labelAdaptersPool = sync.Pool{New: func() interface{} { return &[]mimirpb.LabelAdapter{} }}
	samplesPool       = sync.Pool{New: func() interface{} { return &[]mimirpb.Sample{} }}
	exemplarsPool     = sync.Pool{New: func() interface{} { return &[]mimirpb.Exemplar{} }}
)

// PreallocQueryStreamResponseFromPool returns a new PreallocQueryStreamResponse from the pool.
func PreallocQueryStreamResponseFromPool() *PreallocQueryStreamResponse {
	return streamRespPool.Get().(*PreallocQueryStreamResponse)
}

// ReusePreallocQueryStreamResponse returns a PreallocQueryStreamResponse to the pool.
func ReusePreallocQueryStreamResponse(m *PreallocQueryStreamResponse) {
	for i := 0; i < len(m.Chunkseries); i++ {
		reuseTimeChunkSeries(&m.Chunkseries[i])
	}
	for i := 0; i < len(m.Timeseries); i++ {
		reuseTimeseries(&m.Timeseries[i])
	}
	m.Timeseries = m.Timeseries[:0]
	m.Chunkseries = m.Chunkseries[:0]
	streamRespPool.Put(m)
}

func timeChunkSeriesFromPool() TimeSeriesChunk {
	return TimeSeriesChunk{
		Labels: *(labelAdaptersPool.Get().(*[]mimirpb.LabelAdapter)),
		Chunks: *(chunksPool.Get().(*[]Chunk)),
	}
}

func mimirTimeSeriesFromPool() mimirpb.TimeSeries {
	return mimirpb.TimeSeries{
		Labels:    *(labelAdaptersPool.Get().(*[]mimirpb.LabelAdapter)),
		Samples:   *(samplesPool.Get().(*[]mimirpb.Sample)),
		Exemplars: *(exemplarsPool.Get().(*[]mimirpb.Exemplar)),
	}
}

func reuseTimeChunkSeries(ts *TimeSeriesChunk) {
	chunks := ts.Chunks[:0]
	chunksPool.Put(&chunks)
	ts.Chunks = nil

	labels := ts.Labels[:0]
	labelAdaptersPool.Put(&labels)
	ts.Labels = nil
}

func reuseTimeseries(ts *mimirpb.TimeSeries) {
	samples := ts.Samples[:0]
	samplesPool.Put(&samples)
	ts.Samples = nil

	exemplars := ts.Exemplars[:0]
	exemplarsPool.Put(&exemplars)
	ts.Exemplars = nil

	labels := ts.Labels[:0]
	labelAdaptersPool.Put(&labels)
	ts.Labels = nil
}
