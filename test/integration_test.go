// +build integration

// package integration implement integration tests.
package integration

import (
	"bytes"
	"flag"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/rpc/json"
)

var (
        host = flag.String("host", "localhost", "Host to connect to.")
	port         = flag.Int("port", 8080, "Port to connect to.")
	rpcTimeout   = flag.Duration("rpc_timeout", 1*time.Minute, "Client-side RPC timeout.")
	numRequests  = flag.Int("num_requests", 10000, "Number of requests to make.")
	keySpace     = flag.Int("key_space_size", 600, "Size of a key space to probe.")
	parallelReqs = flag.Int("reqs_in_parallel", 20, "Request parallelization.")
)

func init() {
  flag.Parse()
}

type GetArgs struct {
	Key string
}

type GetReply struct {
	Value string
}

func TestRandomKeys(t *testing.T) {
	client := http.Client{Timeout: *rpcTimeout}
	url := fmt.Sprintf("http://%s:%d/rpc", *host, *port)

	throttler := make(chan struct{}, *parallelReqs)
	for i := 0; i < *parallelReqs; i++ {
		throttler <- struct{}{}
	}

	var cacheHits, cacheMisses, rpcErrors int32

	for i := 0; i < *numRequests; i++ {
		<-throttler
		go func(n int) {
			defer func() { throttler <- struct{}{} }()

			key := fmt.Sprintf("key%d", rand.Intn(*keySpace))
			bs, _ := json.EncodeClientRequest("CacheSrv.Get", &GetArgs{Key: key})
			req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(bs))
			req.Header.Set("Content-Type", "application/json")
			if err != nil {
				t.Fatal(err)
			}

			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			r := new(GetReply)
			err = json.DecodeClientResponse(res.Body, &r)
			if err != nil {
				if strings.Contains(err.Error(), "record not found") {
					atomic.AddInt32(&cacheMisses, 1)
				} else {
					atomic.AddInt32(&rpcErrors, 1)
				}
			} else {
				atomic.AddInt32(&cacheHits, 1)
			}
		}(i)
	}
	if float32(rpcErrors)/float32(*numRequests) > 0.1 {
		t.Errorf("too many RPC errors: %d", rpcErrors)
	}
	if float32(cacheMisses)/float32(cacheHits) > 0.2 {
		t.Errorf("too many cache misses: %d/%d (hits: %d/%d)", cacheMisses, *numRequests, cacheHits, *numRequests)
	}
}
