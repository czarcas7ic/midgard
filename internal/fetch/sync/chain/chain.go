// Package chain provides a blockchain client.
package chain

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/pascaldekloe/metrics"
	rpchttp "github.com/tendermint/tendermint/rpc/client/http"
	coretypes "github.com/tendermint/tendermint/rpc/core/types"
	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/util/miderr"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
	"gitlab.com/thorchain/midgard/internal/util/timer"
)

var logger = midlog.LoggerForModule("chain")

func init() {
	metrics.MustHelp("midgard_chain_cursor_height", "The Tendermint sequence identifier that is next in line.")
	metrics.MustHelp("midgard_chain_height", "The latest Tendermint sequence identifier reported by the node.")
}

// Block is a chain record.
type Block struct {
	Height  int64                         `json:"height"`
	Time    time.Time                     `json:"time"`
	Hash    []byte                        `json:"hash"`
	Results *coretypes.ResultBlockResults `json:"results"`
}

// Client provides Tendermint access.
type Client struct {
	ctx context.Context

	// Single RPC access
	client *rpchttp.HTTP

	// Parallel / batched access
	batchClients []*rpchttp.BatchHTTP

	batchSize   int // divisible by parallelism
	parallelism int
}

func (c *Client) FetchSingle(height int64) (*coretypes.ResultBlockResults, error) {
	return c.client.BlockResults(c.ctx, &height)
}

func (c *Client) BatchSize() int {
	return c.batchSize
}

// NewClient configures a new instance. Timeout applies to all requests on endpoint.
func NewClient(ctx context.Context) (*Client, error) {
	cfg := &config.Global
	var timeout time.Duration = cfg.ThorChain.ReadTimeout.Value()

	endpoint, err := url.Parse(cfg.ThorChain.TendermintURL)
	if err != nil {
		logger.FatalE(err, "Exit on malformed Tendermint RPC URL")
	}

	batchSize := cfg.ThorChain.FetchBatchSize
	parallelism := cfg.ThorChain.Parallelism
	if batchSize%parallelism != 0 {
		logger.FatalF("BatchSize=%d must be divisible by Parallelism=%d", batchSize, parallelism)
	}

	// need the path separate from the URL for some reason
	path := endpoint.Path
	endpoint.Path = ""
	remote := endpoint.String()

	var client *rpchttp.HTTP
	var batchClients []*rpchttp.BatchHTTP
	for i := 0; i < parallelism; i++ {
		// rpchttp.NewWithTimeout rounds to seconds for some reason
		client, err = rpchttp.NewWithClient(remote, path, &http.Client{Timeout: timeout})
		if err != nil {
			return nil, fmt.Errorf("tendermint RPC client instantiation: %w", err)
		}
		batchClients = append(batchClients, client.NewBatch())
	}

	return &Client{
		ctx:          ctx,
		client:       client,
		batchClients: batchClients,
		batchSize:    batchSize,
		parallelism:  parallelism,
	}, nil
}

func (c *Client) FirstBlockHash() (hash string, err error) {
	block := Block{}
	err = c.fetchBlock(&block, 1)
	if err != nil {
		return "", err
	}
	return db.PrintableHash(string(block.Hash)), nil
}

// Fetch Single block details
func (c *Client) GetBlock(height *int64) (*coretypes.ResultBlock, error) {
	return c.client.Block(c.ctx, height)
}

// Fetch the summary of the chain: latest height, node address, ...
func (c *Client) RefreshStatus() (rs *coretypes.ResultStatus, err error) {
	cfg := &config.Global
	for i := 0; i < cfg.ThorChain.MaxStatusRetries; i++ {
		rs, err = c.client.Status(c.ctx)
		if err == nil {
			return rs, nil
		}
		logger.ErrorE(err, "tendermint RPC status")
		time.Sleep(time.Duration(cfg.ThorChain.StatusRetryBackoff))
	}
	return nil, fmt.Errorf("tendermint RPC status: %w", err)
}

var (
	fetchTimerBatch    = timer.NewTimer("block_fetch_batch")
	fetchTimerParallel = timer.NewTimer("block_fetch_parallel")
	fetchTimerSingle   = timer.NewTimer("block_fetch_single")
)

type Iterator struct {
	c                *Client
	nextBatchStart   int64
	finalBlockHeight int64
	batch            []Block
}

func (c *Client) Iterator(startHeight, finalBlockHeight int64) Iterator {
	return Iterator{
		c:                c,
		nextBatchStart:   startHeight,
		finalBlockHeight: finalBlockHeight,
	}
}

func (i *Iterator) Next() (*Block, error) {
	if len(i.batch) == 0 {
		hasMore, err := i.nextBatch()
		if err != nil || !hasMore {
			return nil, err
		}
	}
	ret := &i.batch[0]
	i.batch = i.batch[1:]
	return ret, nil
}

func (i *Iterator) nextBatch() (hasMore bool, err error) {
	if len(i.batch) != 0 {
		return false, miderr.InternalErr("Batch still filled")
	}
	if i.finalBlockHeight < i.nextBatchStart {
		return false, nil
	}

	batchSize := int64(i.c.batchSize)
	parallelism := i.c.parallelism

	remainingOnChain := i.finalBlockHeight - i.nextBatchStart + 1
	if remainingOnChain < batchSize {
		batchSize = remainingOnChain
		parallelism = 1
	}
	i.batch = make([]Block, batchSize)
	err = i.c.fetchBlocksParallel(i.batch, i.nextBatchStart, parallelism)
	i.nextBatchStart += batchSize
	return true, err
}

func (c *Client) fetchBlock(block *Block, height int64) error {
	defer fetchTimerSingle.One()()

	info, err := c.client.BlockchainInfo(c.ctx, height, height)
	if err != nil {
		return fmt.Errorf("BlockchainInfo for %d, failed: %w", height, err)
	}

	if len(info.BlockMetas) != 1 {
		return fmt.Errorf("BlockchainInfo for %d, wrong number of results: %d",
			height, len(info.BlockMetas))
	}

	header := &info.BlockMetas[0].Header
	if header.Height != height {
		return fmt.Errorf("BlockchainInfo for %d, wrong height: %d", height, header.Height)
	}

	block.Height = height
	block.Time = header.Time
	block.Hash = []byte(info.BlockMetas[0].BlockID.Hash)

	block.Results, err = c.client.BlockResults(c.ctx, &block.Height)
	if err != nil {
		return fmt.Errorf("BlockResults for %d, failed: %w", height, err)
	}

	// Validate that heights in the response match the request
	if block.Height != height || block.Results.Height != height {
		return fmt.Errorf("BlockResults for %d, got Height=%d Results.Height=%d",
			height, block.Height, block.Results.Height)
	}

	return nil
}

func (c *Client) fetchBlocks(clientIdx int, batch []Block, height int64) error {
	// Note: n > 1 is required
	n := len(batch)
	var err error
	client := c.batchClients[clientIdx]
	defer fetchTimerBatch.Batch(n)()

	last := height + int64(n) - 1
	infos := make([]*coretypes.ResultBlockchainInfo, n)
	for i := 0; i < n; i++ {
		h := height + int64(i)
		// Note(huginn): we could do "batched batched" request: asking for 20 BlockchainInfos
		// at a time. Measurements suggest that this wouldn't improve the speed, the BlockchainInfo
		// requests take about 4% of the BlockResults, and it would complicate the logic significantly.
		infos[i], err = client.BlockchainInfo(c.ctx, h, h)
		if err != nil {
			return fmt.Errorf("BlockchainInfo batch for %d: %w", h, err)
		}
	}
	_, err = client.Send(c.ctx)
	if err != nil {
		return fmt.Errorf("BlockchainInfo batch Send for %d-%d: %w", height, last, err)
	}

	for i, info := range infos {
		h := height + int64(i)

		if len(info.BlockMetas) != 1 {
			return fmt.Errorf("BlockchainInfo batch for %d, wrong number of results: %d",
				height, len(info.BlockMetas))
		}

		header := &info.BlockMetas[0].Header
		if header.Height != h {
			return fmt.Errorf("BlockchainInfo for %d, wrong height: %d", h, header.Height)
		}

		block := &batch[i]
		block.Height = header.Height
		block.Time = header.Time
		block.Hash = []byte(info.BlockMetas[0].BlockID.Hash)
	}

	for i := range batch {
		block := &batch[i]
		block.Results, err = client.BlockResults(c.ctx, &block.Height)
		if err != nil {
			return fmt.Errorf("BlockResults batch for %d: %w", block.Height, err)
		}
	}

	_, err = client.Send(c.ctx)
	if err != nil {
		return fmt.Errorf("BlockResults batch Send for %d-%d: %w", height, last, err)
	}

	// Validate that heights in the response match the request
	for i := range batch {
		h := height + int64(i)
		block := &batch[i]
		if block.Height != h || block.Results.Height != h {
			return fmt.Errorf("BlockResults batch for %d, got Height=%d Results.Height=%d",
				h, block.Height, block.Results.Height)
		}
	}

	return nil
}

func (c *Client) fetchBlocksParallel(batch []Block, height int64, parallelism int) error {
	n := len(batch)
	if n == 1 {
		return c.fetchBlock(&batch[0], height)
	}

	if parallelism == 1 {
		return c.fetchBlocks(0, batch, height)
	}

	k := n / parallelism
	if k*parallelism != n {
		return fmt.Errorf("batch size %d not divisible into %d parallel parts", n, parallelism)
	}

	defer fetchTimerParallel.Batch(n)()

	done := make(chan error, parallelism)
	for i := 0; i < parallelism; i++ {
		clientIdx := i
		start := i * k
		go func() {
			err := c.fetchBlocks(clientIdx, batch[start:start+k], height+int64(start))
			done <- err
		}()
	}

	var err error
	for i := 0; i < parallelism; i++ {
		e := <-done
		if e != nil {
			err = e
		}
	}
	return err
}
