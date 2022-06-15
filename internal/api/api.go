// Package api provides the HTTP interface.
package api

import (
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/julienschmidt/httprouter"
	"github.com/pascaldekloe/metrics"
	"github.com/rs/zerolog/hlog"
	"github.com/rs/zerolog/log"

	"gitlab.com/thorchain/midgard/config"
	"gitlab.com/thorchain/midgard/internal/graphql"
	"gitlab.com/thorchain/midgard/internal/graphql/generated"
	"gitlab.com/thorchain/midgard/internal/timeseries/stat"
	"gitlab.com/thorchain/midgard/internal/util/midlog"
	"gitlab.com/thorchain/midgard/internal/util/timer"
	"gitlab.com/thorchain/midgard/internal/websocket"
)

// Handler serves the entire API.
var Handler http.Handler

func addMeasured(router *httprouter.Router, url string, handler httprouter.Handle) {
	reg, err := regexp.Compile("[^a-zA-Z0-9]+")
	if err != nil {
		panic("Bad constant url regex.")
	}
	simplifiedURL := reg.ReplaceAllString(url, "_")
	t := timer.NewTimer("serving" + simplifiedURL)

	router.Handle(
		http.MethodGet, url,
		func(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
			m := t.One()
			handler(w, r, ps)
			m()
		})
}

const proxiedPrefix = "/v2/thorchain/"

// InitHandler inits API main handler
func InitHandler(nodeURL string, proxiedWhitelistedEndpoints []string) {
	router := httprouter.New()

	Handler = loggerHandler(corsHandler(router))

	// apply some navigation pointers
	router.HandleMethodNotAllowed = true
	router.HandleOPTIONS = true
	router.HandlerFunc(http.MethodGet, "/", serveRoot)

	router.HandlerFunc(http.MethodGet, "/v2/debug/metrics", metrics.ServeHTTP)
	router.HandlerFunc(http.MethodGet, "/v2/debug/timers", timer.ServeHTTP)
	router.HandlerFunc(http.MethodGet, "/v2/debug/usd", stat.ServeUSDDebug)
	router.Handle(http.MethodGet, "/v2/debug/block/:id", debugBlock)

	for _, endpoint := range proxiedWhitelistedEndpoints {
		midgardPath := proxiedPrefix + endpoint
		addMeasured(router, midgardPath, proxyHandler(nodeURL))
	}

	router.HandlerFunc(http.MethodGet, "/v2/doc", serveDoc)

	// version 1
	addMeasured(router, "/v2/actions", jsonActions)
	addMeasured(router, "/v2/health", jsonHealth)
	addMeasured(router, "/v2/history/swaps", jsonSwapHistory)
	addMeasured(router, "/v2/history/depths/:pool", jsonDepths)
	addMeasured(router, "/v2/history/earnings", jsonEarningsHistory)
	addMeasured(router, "/v2/history/liquidity_changes", jsonLiquidityHistory)
	addMeasured(router, "/v2/history/tvl", jsonTVLHistory)
	addMeasured(router, "/v2/network", jsonNetwork)
	addMeasured(router, "/v2/nodes", jsonNodes)
	addMeasured(router, "/v2/members", jsonMembers)
	addMeasured(router, "/v2/member/:addr", jsonMemberDetails)
	addMeasured(router, "/v2/pools", jsonPools)
	addMeasured(router, "/v2/pool/:pool", jsonPool)
	addMeasured(router, "/v2/pool/:pool/stats", jsonPoolStats)
	router.Handle(http.MethodGet, "/v2/stats", cachedJsonStats())
	addMeasured(router, "/v2/swagger.json", jsonSwagger)
	addMeasured(router, "/v2/thorname/lookup/:name", jsonTHORName)
	addMeasured(router, "/v2/thorname/rlookup/:address", jsonTHORNameAddress)
	addMeasured(router, "/v2/thorname/owner/:address", jsonTHORNameOwner)
	addMeasured(router, "/v2/websocket", websocket.Handler)
	if config.Global.EventRecorder.OnTransferEnabled {
		addMeasured(router, "/v2/balance/:address", jsonBalance)
	}

	// version 2 with GraphQL
	router.HandlerFunc(http.MethodGet, "/v2/graphql", playground.Handler("Midgard Playground", "/v2"))
	router.Handle(http.MethodPost, "/v2", serverV2())

	router.PanicHandler = panicHandler
}

func panicHandler(w http.ResponseWriter, r *http.Request, err interface{}) {
	log.Error().Interface("error", err).Str("path", r.URL.Path).Msg("panic http handler")
	w.WriteHeader(http.StatusInternalServerError)
}

func serveDoc(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "./openapi/generated/doc.html")
}

func serverV2() httprouter.Handle {
	h := handler.NewDefaultServer(generated.NewExecutableSchema(generated.Config{Resolvers: &graphql.Resolver{}}))
	return func(w http.ResponseWriter, req *http.Request, _ httprouter.Params) {
		h.ServeHTTP(w, req)
	}
}

func serveRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain;charset=UTF-8")

	// Discarding errors
	_, _ = io.WriteString(w, `# THORChain Midgard

Welcome to the HTTP interface.
`)
}

func proxyHandler(nodeURL string) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		targetPath := strings.TrimPrefix(r.URL.Path, proxiedPrefix)
		url, err := url.Parse(nodeURL + "/" + targetPath)
		if err != nil {
			http.NotFound(w, r)
		}

		proxy := httputil.NewSingleHostReverseProxy(url)
		proxy.Director = func(req *http.Request) {
			req.Header.Add("X-Forwarded-Host", req.Host)
			req.Header.Add("X-Origin-Host", url.Host)
			req.Host = url.Host
			req.URL.Scheme = url.Scheme
			req.URL.Host = url.Host
			req.URL.Path = url.Path
		}
		proxy.ServeHTTP(w, r)
	}
}

func corsHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, proxiedPrefix) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}
		h.ServeHTTP(w, r)
	})
}

func loggerHandler(h http.Handler) http.Handler {
	logger := midlog.LoggerForModule("http")

	// simillar to hlog.NewHandler
	setLoggerInContext := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Create a copy of the logger (including internal context slice)
			// to prevent data race when using UpdateContext.
			l := logger.GetZeroLogger().With().Logger()
			r = r.WithContext(l.WithContext(r.Context()))
			next.ServeHTTP(w, r)
		})
	}

	logSummaryAfter := hlog.AccessHandler(func(r *http.Request, status, size int, duration time.Duration) {
		hlog.FromRequest(r).Debug().
			Str("method", r.Method).
			Str("url", r.URL.String()).
			Int("status", status).
			Int("size", size).
			Dur("duration_ms", duration).
			Msg("Access")
	})

	remoteAddrHandler := hlog.RemoteAddrHandler("ip")
	userAgentHandler := hlog.UserAgentHandler("user_agent")
	refererHandler := hlog.RefererHandler("referer")
	requestIDHandler := hlog.RequestIDHandler("req_id", "X-Request-Id")

	return setLoggerInContext(
		logSummaryAfter(
			remoteAddrHandler(
				userAgentHandler(
					refererHandler(
						requestIDHandler(h))))))
}
