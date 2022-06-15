package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/julienschmidt/httprouter"
	log "github.com/rs/zerolog/log"
	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/db"
	"gitlab.com/thorchain/midgard/internal/timeseries"
	"golang.org/x/net/websocket"
)

////////////////////////////////////////////////////////////////////////////////////////
// Events
////////////////////////////////////////////////////////////////////////////////////////

type ErrorEvent struct {
	Error string `json:"error"`
}

type BlockEvent struct {
	Height int64   `json:"height"`
	Prices []Price `json:"prices"`
}

type Price struct {
	Asset     string  `json:"asset"`
	RunePrice float64 `json:"runePrice"`
}

////////////////////////////////////////////////////////////////////////////////////////
// Handler
////////////////////////////////////////////////////////////////////////////////////////

var httpHandler http.Handler = websocket.Handler(handler)

// Handler is the httprouter.Handle function that wraps the underlying websocket.
func Handler(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	if !config.Global.Websockets.Enable {
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte("websockets not enabled"))
	} else {
		httpHandler.ServeHTTP(w, r)
	}
}

func handler(ws *websocket.Conn) {
	// setup
	websocketReader := json.NewDecoder(ws)
	defer ws.Close()

	// read the query
	q := &Query{websocketWriter: json.NewEncoder(ws)}
	err := websocketReader.Decode(&q)
	switch {
	case err == io.EOF:
		return
	case err != nil:
		log.Info().Err(err).Msg("bad query")
		_ = q.websocketWriter.Encode(ErrorEvent{err.Error()})
		return
	}

	// handle the query
	err = q.handle()
	if err != nil {
		log.Debug().Err(err).Msg("query error")
		_ = q.websocketWriter.Encode(ErrorEvent{err.Error()})
	}
}

////////////////////////////////////////////////////////////////////////////////////////
// Query
////////////////////////////////////////////////////////////////////////////////////////

const (
	MethodSubscribePriceV1 = "subscribePriceV1"
)

type Query struct {
	websocketWriter *json.Encoder
	ctx             context.Context

	Method string   `json:"method"`
	Assets []string `json:"assets"`
}

func (q *Query) handle() error {
	switch q.Method {
	case MethodSubscribePriceV1:
		return q.subscribePriceV1()
	default:
		return errors.New("unknown method")
	}
}

func (q *Query) send(event interface{}) error {
	return q.websocketWriter.Encode(event)
}

// ------------------------------ handlers ------------------------------

func (q *Query) subscribePriceV1() error {
	if len(q.Assets) == 0 {
		return errors.New("must provide list of assets")
	}

	assets := map[string]bool{}
	last := map[string]timeseries.PoolDepths{}
	event := &BlockEvent{}
	for _, asset := range q.Assets {
		assets[asset] = true
	}

	for {
		// collect price changes for subscribe assets
		state := timeseries.Latest.GetState()
		event.Height = state.Height
		event.Prices = event.Prices[:0]
		for asset, info := range state.Pools {
			if !assets[asset] {
				continue
			}
			if lastInfo, ok := last[asset]; ok && lastInfo == info {
				continue
			}
			last[asset] = info
			event.Prices = append(event.Prices, Price{asset, info.AssetPrice()})
		}

		// write the block event
		err := q.websocketWriter.Encode(event)
		if err != nil {
			return err
		}
		db.WaitBlock()
	}
}
