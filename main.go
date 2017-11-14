package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dilyevsky/httplru/cache"
	"github.com/gorilla/rpc"
	"github.com/gorilla/rpc/json"
)

var (
	port             = flag.Int("port", 8080, "Port to export api on.")
	maxCachedEntries = flag.Int("max_cache_size", 1000, "Maximum number of entries in the cache.")
	ttl              = flag.Duration("ttl", 1*time.Hour, "Entires are expired after this period.")
	maxRPCs          = flag.Int("max_concurrent_reqs", 100, "Maximum number of RPCs that could be processed concurrently.")
	rpcTimeout       = flag.Duration("rpc_timeout", 1*time.Minute, "Server-side RPC deadline.")
	testDataPath     = flag.String("test_data", "", "If non-empty, points to a file of pre-populate data of format key:value (line-separated).")
)

// CacheSrv serves LRU Cache operations via RPCs.
type CacheSrv struct {
	cache       cache.LRUCache
	allowedReqs chan struct{}
}

// GetArgs contains arguments for Get RPC.
type GetArgs struct {
	Key string
}

// GetArgs contains reply to Get RPC.
type GetReply struct {
	Value string
}

func (s *CacheSrv) Get(r *http.Request, args *GetArgs, reply *GetReply) error {
	select {
	case <-s.allowedReqs:
	case <-time.After(*rpcTimeout):
		return errors.New("timed out: too many ongoing requests")
	}
	defer func() { s.allowedReqs <- struct{}{} }() // Reclaim resource on exit.

	v, ok := s.cache.Get([]byte(args.Key))
	if !ok {
		return fmt.Errorf("record not found for: %v", args.Key)
	}
	reply.Value = string(v)
	return nil
}

// newThrottler creates throttler that gives out resources up to allowed.
// Resources must be returned by re-sending on the channel.
func newThrottler(allowed int) chan struct{} {
	ch := make(chan struct{}, allowed)
	for i := 0; i < allowed; i++ {
		ch <- struct{}{}
	}
	return ch
}

func populateTestData(c cache.LRUCache, path string) {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		ss := strings.Split(scanner.Text(), ":")
		if len(ss) != 2 {
			log.Fatal("invalid test data format")
		}

		c.Add([]byte(ss[0]), []byte(ss[1]))
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}

}

func main() {
	flag.Parse()

	opts := badger.DefaultOptions
	badgerDir := "/tmp/badger"
	// Remove badger db files that might be present from previous run.
	// TODO(dilyevsky): Better way would be to re-load snapshot and
	// re-build cache in-memory state from that.
	os.RemoveAll(badgerDir)
	opts.Dir = badgerDir
	opts.ValueDir = badgerDir
	db, err := badger.Open(opts)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	c := cache.NewLRUCache(db, *maxCachedEntries, *ttl)
	if *testDataPath != "" {
		populateTestData(c, *testDataPath)
	}

	s := rpc.NewServer()
	s.RegisterCodec(json.NewCodec(), "application/json")
	s.RegisterService(&CacheSrv{c, newThrottler(*maxRPCs)}, "")
	http.Handle("/rpc", s)
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), nil))
}
