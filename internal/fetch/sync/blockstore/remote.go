package blockstore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"

	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
)

func (b *BlockStore) updateFromRemote(ctx context.Context) {
	if b.cfg.Remote == "" {
		return
	}

	defer b.cleanUp()

	wasUpdated := false

	localChunks, err := b.getLocalChunkNames()
	if err != nil {
		logger.ErrorE(err, "Error updating from remote")
		return
	}
	acceptableHashVals := b.readChunkHashes()
	n := float32(len(acceptableHashVals))
	for i, chunkHash := range acceptableHashVals {
		if ctx.Err() != nil {
			logger.Warn("Fetch interrupted")
			break
		}
		//
		chunkBlockHeight, err := strconv.ParseInt(string(chunkHash.name), 10, 64)
		if err != nil {
			logger.ErrorF("Can't parse blockstore height")
		}
		if db.ConfigHasGenesis() && chunkBlockHeight < db.GenesisData.GetGenesisHeight() {
			continue
		}
		if localChunks[chunkHash.name] {
			continue
		}
		logger.InfoF("  [%.2f%%] fetching chunk: %v", 100*float32(i)/n, chunkHash.name)
		wasUpdated = true
		if err := b.fetchChunk(chunkHash); err != nil {
			if err == io.EOF {
				logger.ErrorF("Chunk not found %v", chunkHash)
				break
			}
			logger.ErrorE(err, "Error updating from remote")
			return
		}
	}

	if wasUpdated {
		logger.InfoF("Updating from remote done")
	}
}

func (b *BlockStore) fetchChunk(aChunk *chunk) error {
	resp, err := http.Get(b.remotePath(aChunk.name))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return io.EOF
	}
	defer resp.Body.Close()
	if err := b.startNewFile(); err != nil {
		return err
	}
	b.blockWriter = b.currentFile
	sha256 := sha256.New()
	if _, err := io.Copy(io.MultiWriter(b.currentFile, sha256), resp.Body); err != nil {
		return err
	}
	if actualHash := hex.EncodeToString(sha256.Sum(nil)); aChunk.hash != actualHash {
		return miderr.InternalErrF("BlockStore: Chunk hash mismatch, expected %v, received %v", aChunk, actualHash)
	}
	if err := b.finalizeChunk(aChunk.name); err != nil {
		return err
	}

	return nil
}
