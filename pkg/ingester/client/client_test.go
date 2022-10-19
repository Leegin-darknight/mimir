// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/ingester/client/client_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package client

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"

	"github.com/grafana/mimir/pkg/mimirpb"
	"github.com/grafana/mimir/pkg/util"
)

// TestMarshall is useful to try out various optimisation on the unmarshalling code.
func TestMarshall(t *testing.T) {
	const numSeries = 10
	recorder := httptest.NewRecorder()
	{
		req := mimirpb.WriteRequest{}
		for i := 0; i < numSeries; i++ {
			req.Timeseries = append(req.Timeseries, mimirpb.PreallocTimeseries{
				TimeSeries: &mimirpb.TimeSeries{
					Labels: []mimirpb.LabelAdapter{
						{Name: "foo", Value: strconv.Itoa(i)},
					},
					Samples: []mimirpb.Sample{
						{TimestampMs: int64(i), Value: float64(i)},
					},
				},
			})
		}
		err := util.SerializeProtoResponse(recorder, &req, util.RawSnappy)
		require.NoError(t, err)
	}

	{
		const (
			tooSmallSize = 1
			plentySize   = 1024 * 1024
		)
		req := mimirpb.WriteRequest{}
		_, err := util.ParseProtoReader(context.Background(), recorder.Body, recorder.Body.Len(), tooSmallSize, nil, &req, util.RawSnappy)
		require.Error(t, err)
		_, err = util.ParseProtoReader(context.Background(), recorder.Body, recorder.Body.Len(), plentySize, nil, &req, util.RawSnappy)
		require.NoError(t, err)
		require.Equal(t, numSeries, len(req.Timeseries))
	}
}

func TestPreallocQueryStreamResponseUnmarshal(t *testing.T) {
	var preAllocResp PreallocQueryStreamResponse

	const (
		numSeries          = 10
		numChunksPerSeries = 100
	)
	resp := &QueryStreamResponse{
		Chunkseries: []TimeSeriesChunk{
			{
				Labels: []mimirpb.LabelAdapter{
					{Name: "foo", Value: "bar"},
				},
			},
		},
		Timeseries: []mimirpb.TimeSeries{
			{
				Labels: []mimirpb.LabelAdapter{
					{Name: "foo", Value: "bar"},
				},
			},
		},
	}
	for i := 0; i < numSeries; i++ {
		ss := TimeSeriesChunk{
			Labels: []mimirpb.LabelAdapter{
				{Name: "foo", Value: fmt.Sprintf("bar-%d", i)},
			},
		}
		for j := 0; j < numChunksPerSeries; j++ {
			ss.Chunks = append(ss.Chunks, Chunk{
				StartTimestampMs: int64(j) * 100,
				EndTimestampMs:   int64(j)*100 + 10,
				Data:             []byte(fmt.Sprintf("chunk-%d", j)),
			})
		}
		resp.Chunkseries = append(resp.Chunkseries, ss)
	}

	b, err := proto.Marshal(resp)
	require.NoError(t, err)
	require.True(t, len(b) > 0)

	err = proto.Unmarshal(b, &preAllocResp)
	require.NoError(t, err)

	require.Equal(t, resp.String(), preAllocResp.QueryStreamResponse.String())
}
