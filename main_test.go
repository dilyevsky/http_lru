package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/dgraph-io/badger"
	"github.com/dilyevsky/httplru/cache"
	"github.com/gorilla/rpc"
	"github.com/gorilla/rpc/json"
)

func getExisting(s *rpc.Server, key string, wantValue string) func(t *testing.T) {
	return func(t *testing.T) {
		jsonBytes, _ := json.EncodeClientRequest("CacheSrv.Get", &GetArgs{Key: key})
		httpReq, _ := http.NewRequest("POST", "http://example.com/rpc", bytes.NewBuffer(jsonBytes))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		s.ServeHTTP(w, httpReq)

		var r GetReply
		err := json.DecodeClientResponse(w.Body, &r)
		if err != nil {
			t.Error(err)
		}
                if r.Value != wantValue {
                    t.Errorf("unexpected value. want: %v, got: %v", wantValue, r.Value)
                }
	}
}

func getMissing(s *rpc.Server, key string) func(t *testing.T) {
	return func(t *testing.T) {
		jsonBytes, _ := json.EncodeClientRequest("CacheSrv.Get", &GetArgs{Key: key})
		httpReq, _ := http.NewRequest("POST", "http://example.com/rpc", bytes.NewBuffer(jsonBytes))
		httpReq.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		s.ServeHTTP(w, httpReq)

		var r GetReply
		wantErr := fmt.Sprintf("record not found for: %v", key)
		gotErr := json.DecodeClientResponse(w.Body, &r)
		if gotErr == nil {
			t.Error("expected non-nil error")
		} else if gotErr.Error() != wantErr {
			t.Errorf("unexpected error: want: %v, got: %v", wantErr, gotErr)
		}

	}
}

func runTests(c cache.LRUCache, t *testing.T) {
	s := rpc.NewServer()
	s.RegisterCodec(json.NewCodec(), "application/json")
	s.RegisterService(&CacheSrv{c, newThrottler(1)}, "")

	c.Add([]byte("testKey1"), []byte("testVal1"))
	c.Add([]byte("testKey2"), []byte("testVal2"))
	c.Add([]byte("testKey3"), []byte("testVal3")) // testKey1 is evicted.

	t.Run("Get", getExisting(s, "testKey3", "testVal3"))
	t.Run("Get", getExisting(s, "testKey2", "testVal2"))
	t.Run("GetMissingKey", getMissing(s, "testKey1"))

	// testKey3 was touched last so it's evicted.
	c.Add([]byte("testKey4"), []byte("testVal4"))
	t.Run("GetMissingKey", getMissing(s, "testKey3"))

	// [testKey4 testKey2]

	// Duplicate adds should bump the key in front.
	c.Add([]byte("testKey2"), []byte("testVal2"))
	c.Add([]byte("testKey5"), []byte("testVal5"))
	t.Run("GetMissingKey", getMissing(s, "testKey4"))
}

func TestSimpleRLU(t *testing.T) {
	c := cache.NewSimpleLRUCache(2, time.Second)

	runTests(c, t)
}

func TestBadgerRLU(t *testing.T) {
	opts := badger.DefaultOptions
	opts.Dir = "/tmp/badger-test"
	opts.ValueDir = "/tmp/badger-test"
	db, err := badger.Open(opts)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	c := cache.NewLRUCache(db, 2, time.Second)

	runTests(c, t)
}
