// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/tsdb/bucketindex/updater_test.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package bucketindex

import (
	"bytes"
	"context"
	"path"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
	"github.com/thanos-io/thanos/pkg/block"
	"github.com/thanos-io/thanos/pkg/block/metadata"

	"github.com/grafana/mimir/pkg/storage/bucket"
	mimir_tsdb "github.com/grafana/mimir/pkg/storage/tsdb"
	"github.com/grafana/mimir/pkg/storage/tsdb/testutil"
)

func TestUpdater_UpdateIndex(t *testing.T) {
	const userID = "user-1"

	bkt, _ := testutil.PrepareFilesystemBucket(t)

	ctx := context.Background()
	logger := log.NewNopLogger()

	// Generate the initial index.
	bkt = BucketWithGlobalMarkers(bkt)
	block1 := testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 10, 20, nil)
	testutil.MockNoCompactMark(t, bkt, userID, block1.BlockMeta) // no-compact mark is ignored by bucket index updater.
	block2 := testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 20, 30, map[string]string{mimir_tsdb.CompactorShardIDExternalLabel: "1_of_5"})
	block2Mark := testutil.MockStorageDeletionMark(t, bkt, userID, block2.BlockMeta)

	w := NewUpdater(bkt, userID, nil, logger)
	returnedIdx, _, err := w.UpdateIndex(ctx, nil)
	require.NoError(t, err)
	assertBucketIndexEqual(t, returnedIdx, bkt, userID,
		[]metadata.Meta{block1, block2},
		[]*metadata.DeletionMark{block2Mark})

	// Create new blocks, and update the index.
	block3 := testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 30, 40, map[string]string{"aaa": "bbb"})
	block4 := testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 40, 50, map[string]string{mimir_tsdb.CompactorShardIDExternalLabel: "2_of_5"})
	block4Mark := testutil.MockStorageDeletionMark(t, bkt, userID, block4.BlockMeta)

	returnedIdx, _, err = w.UpdateIndex(ctx, returnedIdx)
	require.NoError(t, err)
	assertBucketIndexEqual(t, returnedIdx, bkt, userID,
		[]metadata.Meta{block1, block2, block3, block4},
		[]*metadata.DeletionMark{block2Mark, block4Mark})

	// Hard delete a block and update the index.
	require.NoError(t, block.Delete(ctx, log.NewNopLogger(), bucket.NewUserBucketClient(userID, bkt, nil), block2.ULID))

	returnedIdx, _, err = w.UpdateIndex(ctx, returnedIdx)
	require.NoError(t, err)
	assertBucketIndexEqual(t, returnedIdx, bkt, userID,
		[]metadata.Meta{block1, block3, block4},
		[]*metadata.DeletionMark{block4Mark})
}

func TestUpdater_UpdateIndex_ShouldSkipPartialBlocks(t *testing.T) {
	const userID = "user-1"

	bkt, _ := testutil.PrepareFilesystemBucket(t)

	ctx := context.Background()
	logger := log.NewNopLogger()

	// Mock some blocks in the storage.
	bkt = BucketWithGlobalMarkers(bkt)
	block1 := testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 10, 20, map[string]string{"hello": "world"})
	block2 := testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 20, 30, map[string]string{mimir_tsdb.CompactorShardIDExternalLabel: "3_of_10"})
	block3 := testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 30, 40, nil)
	block2Mark := testutil.MockStorageDeletionMark(t, bkt, userID, block2.BlockMeta)

	// No compact marks are ignored by bucket index.
	testutil.MockNoCompactMark(t, bkt, userID, block3.BlockMeta)

	// Delete a block's meta.json to simulate a partial block.
	require.NoError(t, bkt.Delete(ctx, path.Join(userID, block3.ULID.String(), metadata.MetaFilename)))

	w := NewUpdater(bkt, userID, nil, logger)
	idx, partials, err := w.UpdateIndex(ctx, nil)
	require.NoError(t, err)
	assertBucketIndexEqual(t, idx, bkt, userID,
		[]metadata.Meta{block1, block2},
		[]*metadata.DeletionMark{block2Mark})

	assert.Len(t, partials, 1)
	assert.True(t, errors.Is(partials[block3.ULID], ErrBlockMetaNotFound))
}

func TestUpdater_UpdateIndex_ShouldSkipBlocksWithCorruptedMeta(t *testing.T) {
	const userID = "user-1"

	bkt, _ := testutil.PrepareFilesystemBucket(t)

	ctx := context.Background()
	logger := log.NewNopLogger()

	// Mock some blocks in the storage.
	bkt = BucketWithGlobalMarkers(bkt)
	block1 := testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 10, 20, nil)
	block2 := testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 20, 30, map[string]string{mimir_tsdb.CompactorShardIDExternalLabel: "55_of_64"})
	block3 := testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 30, 40, nil)
	block2Mark := testutil.MockStorageDeletionMark(t, bkt, userID, block2.BlockMeta)

	// Overwrite a block's meta.json with invalid data.
	require.NoError(t, bkt.Upload(ctx, path.Join(userID, block3.ULID.String(), metadata.MetaFilename), bytes.NewReader([]byte("invalid!}"))))

	w := NewUpdater(bkt, userID, nil, logger)
	idx, partials, err := w.UpdateIndex(ctx, nil)
	require.NoError(t, err)
	assertBucketIndexEqual(t, idx, bkt, userID,
		[]metadata.Meta{block1, block2},
		[]*metadata.DeletionMark{block2Mark})

	assert.Len(t, partials, 1)
	assert.True(t, errors.Is(partials[block3.ULID], ErrBlockMetaCorrupted))
}

func TestUpdater_UpdateIndex_ShouldSkipCorruptedDeletionMarks(t *testing.T) {
	const userID = "user-1"

	bkt, _ := testutil.PrepareFilesystemBucket(t)

	ctx := context.Background()
	logger := log.NewNopLogger()

	// Mock some blocks in the storage.
	bkt = BucketWithGlobalMarkers(bkt)
	block1 := testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 10, 20, nil)
	block2 := testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 20, 30, nil)
	block3 := testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 30, 40, map[string]string{mimir_tsdb.CompactorShardIDExternalLabel: "2_of_7"})
	block2Mark := testutil.MockStorageDeletionMark(t, bkt, userID, block2.BlockMeta)

	// Overwrite a block's deletion-mark.json with invalid data.
	require.NoError(t, bkt.Upload(ctx, path.Join(userID, block2Mark.ID.String(), metadata.DeletionMarkFilename), bytes.NewReader([]byte("invalid!}"))))

	w := NewUpdater(bkt, userID, nil, logger)
	idx, partials, err := w.UpdateIndex(ctx, nil)
	require.NoError(t, err)
	assertBucketIndexEqual(t, idx, bkt, userID,
		[]metadata.Meta{block1, block2, block3},
		[]*metadata.DeletionMark{})
	assert.Empty(t, partials)
}

func TestUpdater_UpdateIndex_NoTenantInTheBucket(t *testing.T) {
	const userID = "user-1"

	ctx := context.Background()
	bkt, _ := testutil.PrepareFilesystemBucket(t)

	for _, oldIdx := range []*Index{nil, {}} {
		w := NewUpdater(bkt, userID, nil, log.NewNopLogger())
		idx, partials, err := w.UpdateIndex(ctx, oldIdx)

		require.NoError(t, err)
		assert.Equal(t, IndexVersion2, idx.Version)
		assert.InDelta(t, time.Now().Unix(), idx.UpdatedAt, 2)
		assert.Len(t, idx.Blocks, 0)
		assert.Len(t, idx.BlockDeletionMarks, 0)
		assert.Empty(t, partials)
	}
}

func TestUpdater_UpdateIndexFromVersion1ToVersion2(t *testing.T) {
	const userID = "user-1"

	bkt, _ := testutil.PrepareFilesystemBucket(t)

	ctx := context.Background()
	logger := log.NewNopLogger()

	// Generate blocks with compactor shard ID.
	bkt = BucketWithGlobalMarkers(bkt)
	block1 := testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 10, 20, map[string]string{mimir_tsdb.CompactorShardIDExternalLabel: "1_of_4"})
	block2 := testutil.MockStorageBlockWithExtLabels(t, bkt, userID, 20, 30, map[string]string{mimir_tsdb.CompactorShardIDExternalLabel: "3_of_4"})

	block1WithoutCompactorShardID := block1
	block1WithoutCompactorShardID.Thanos.Labels = nil

	block2WithoutCompactorShardID := block2
	block2WithoutCompactorShardID.Thanos.Labels = nil

	// Double check that original block1 and block2 still have compactor shards set.
	require.Equal(t, "1_of_4", block1.Thanos.Labels[mimir_tsdb.CompactorShardIDExternalLabel])
	require.Equal(t, "3_of_4", block2.Thanos.Labels[mimir_tsdb.CompactorShardIDExternalLabel])

	// Generate index (this produces V2 index, with compactor shard IDs).
	w := NewUpdater(bkt, userID, nil, logger)
	returnedIdx, _, err := w.UpdateIndex(ctx, nil)
	require.NoError(t, err)
	assertBucketIndexEqual(t, returnedIdx, bkt, userID,
		[]metadata.Meta{block1, block2},
		[]*metadata.DeletionMark{})

	// Now remove Compactor Shard ID from index.
	for _, b := range returnedIdx.Blocks {
		b.CompactorShardID = ""
	}

	// Try to update existing index. Since we didn't change the version, updater will reuse the index, and not update CompactorShardID field.
	returnedIdx, _, err = w.UpdateIndex(ctx, returnedIdx)
	require.NoError(t, err)
	assertBucketIndexEqual(t, returnedIdx, bkt, userID,
		[]metadata.Meta{block1WithoutCompactorShardID, block2WithoutCompactorShardID}, // No compactor shards in bucket index.
		[]*metadata.DeletionMark{})

	// Now set index version to old version 1. Rerunning updater should rebuild index from scratch.
	returnedIdx.Version = IndexVersion1

	returnedIdx, _, err = w.UpdateIndex(ctx, returnedIdx)
	require.NoError(t, err)
	assertBucketIndexEqual(t, returnedIdx, bkt, userID,
		[]metadata.Meta{block1, block2}, // Compactor shards are back.
		[]*metadata.DeletionMark{})
}

func getBlockUploadedAt(t testing.TB, bkt objstore.Bucket, userID string, blockID ulid.ULID) int64 {
	metaFile := path.Join(userID, blockID.String(), block.MetaFilename)

	attrs, err := bkt.Attributes(context.Background(), metaFile)
	require.NoError(t, err)

	return attrs.LastModified.Unix()
}

func assertBucketIndexEqual(t testing.TB, idx *Index, bkt objstore.Bucket, userID string, expectedBlocks []metadata.Meta, expectedDeletionMarks []*metadata.DeletionMark) {
	assert.Equal(t, IndexVersion2, idx.Version)
	assert.InDelta(t, time.Now().Unix(), idx.UpdatedAt, 2)

	// Build the list of expected block index entries.
	var expectedBlockEntries []*Block
	for _, b := range expectedBlocks {
		expectedBlockEntries = append(expectedBlockEntries, &Block{
			ID:               b.ULID,
			MinTime:          b.MinTime,
			MaxTime:          b.MaxTime,
			UploadedAt:       getBlockUploadedAt(t, bkt, userID, b.ULID),
			CompactorShardID: b.Thanos.Labels[mimir_tsdb.CompactorShardIDExternalLabel],
		})
	}

	assert.ElementsMatch(t, expectedBlockEntries, idx.Blocks)

	// Build the list of expected block deletion mark index entries.
	var expectedMarkEntries []*BlockDeletionMark
	for _, m := range expectedDeletionMarks {
		expectedMarkEntries = append(expectedMarkEntries, &BlockDeletionMark{
			ID:           m.ID,
			DeletionTime: m.DeletionTime,
		})
	}

	assert.ElementsMatch(t, expectedMarkEntries, idx.BlockDeletionMarks)
}
