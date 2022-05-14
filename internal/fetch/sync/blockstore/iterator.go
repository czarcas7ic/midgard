package blockstore

import (
	"bufio"
	"io"
	"os"
	"path/filepath"

	"github.com/DataDog/zstd"
	"gitlab.com/thorchain/midgard/internal/fetch/sync/chain"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
)

type Iterator struct {
	blockStore   *BlockStore
	file         *os.File
	zstdReader   io.ReadCloser
	reader       *bufio.Reader
	currentChunk *chunk
	nextHeight   int64
}

func (it *Iterator) Next() (*chain.Block, error) {
	if it.blockStore == nil {
		return nil, nil
	}
	if it.isNextChunkReached() {
		if err := it.cleanupCurrentChunk(); err != nil {
			return nil, err
		}
		if err := it.openNextChunk(); err != nil {
			if err == io.EOF {
				return nil, nil
			}
			return nil, err
		}
	}
	return it.unmarshalNextBlock()
}

func (it *Iterator) isNextChunkReached() bool {
	if it.file == nil {
		return true
	}

	return it.currentChunk.height < it.nextHeight
}

func (it *Iterator) cleanupCurrentChunk() error {
	if it.reader == nil {
		return nil
	}
	if err := it.zstdReader.Close(); err != nil {
		return err
	}
	if err := it.file.Close(); err != nil {
		return err
	}

	it.currentChunk = nil
	it.file = nil
	it.zstdReader = nil
	it.reader = nil

	return nil
}

func (it *Iterator) openNextChunk() error {
	nextChunkPath, err := it.blockStore.findChunkPathForHeight(it.nextHeight)
	if err != nil {
		return err
	}
	f, err := os.Open(nextChunkPath)
	if err != nil {
		return miderr.InternalErrF("BlockStore: unable to open chunk %s: %v", nextChunkPath, err)
	}

	it.file = f
	it.zstdReader = zstd.NewReader(bufio.NewReader(it.file))
	it.reader = bufio.NewReader(it.zstdReader)
	if it.currentChunk, err = NewChunk(filepath.Base(it.file.Name())); err != nil {
		return err
	}
	return nil
}

func (it *Iterator) unmarshalNextBlock() (*chain.Block, error) {
	if it.reader == nil {
		return nil, io.EOF
	}
	for {
		line, err := it.reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				return nil, err
			}
			return nil, miderr.InternalErrF(
				"BlockStore: reached end of file, no block found with height %d", it.nextHeight)
		}
		if !gobLineMatchHeight(line, it.nextHeight) {
			continue
		}
		block, err := GobLineToBlock(line)
		if err != nil {
			return nil, err
		}
		it.nextHeight++
		return block, nil
	}
}
