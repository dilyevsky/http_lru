# LRU cache over HTTP

## Architecture

httplru provides two implementations - fully in-memory LRU and one based on
[BadgerDB](https://github.com/dgraph-io/badger). BadgerDB is an embeddable k/v
store in native Go similar to LevelDB and RocksDB. It is fast, performant and
very easy to use.

The idea behind using LevelDB-like embedded k/v database is too take advantage of
LSM/B+ tree structure that off-loads records to disk as optimal
speed vs memory trade-off.

Service provide /rpc JSON over HTTP endpoint with a single Get method that
retrieves the key (as well as updating its freshness in the LRU) if it is found.
Returns error otherwise.

The LRU algorithm itself consists of doubly-linked list and an index (just a
hash table mapping key to (list pointer, ttl, value) with a simple in-memory
case). When values are accessed, we perform hash lookup and move element (if
present and ttl not expired) to the front of the list. If elements exceed max
cache size, the oldest element (back of the list) is removed.

For k/v databased implementation, we still maintain list in-memory but it only
contains a hash(key) which is used to map to (key, value, ttl) in the k/v store.
Separate hash(key)->listElem hashmap is also maintained in memory to access
the list in O(1) time.

## Maintainability

Package `cache` defines `LRUCache` interface so that service could be easily
with other implementation. I provided simple fully in-memory implementation and
a more complex k/v store-based one based on BadgerDB.

Additionally BadgerDB implementation could be further extended to any other k/v
databases (there isn't one now, but another Go interface layer could be used).

HTTP endpoint is just a simple `CacheSrv` structure defined in main which could
also be extended as needed (potentially moved into a separate package).

## Tests

HTTP endpoint with drop-in cache engine make it easy to unit-test almost the
entire service at once using `httptest` package.

I added a few obvious cases testing addition and retrieval to make sure LRU
functionality works as intended. There's no test for ttl as it would require
setting up time injection so that's a TODO.

For integration, I hammered it with 10K random requests (test key space exceeds
existing keys) and made sure the hit/miss ratios aren't out of whack. More
benchmark-like tests could be easily added.

## Complexity

LRU operations are O(log n) for reads and O(1) for insertions. For reads the
in-memory portion of the query consists index lookup (Golang map is O(1)) and
doubly-linked list operation, both of which are constant time. The k/v store
lookup is O(log n)

For writes, k/v store can drop down to O(n logn) due to write amplification. If
that is of concern another DB based on B+ tree could be used for extra large
loads.o

Simple in-memory implementation is O(1).

## Instructions

Running service:

```
make run
```

That will spin out (build if necessary) a docker container with service
exposed on localhost:8080/rpc pre-loaded with test data (See `test/test_data`).

Query by:

```
curl -X POST -H "Content-Type: application/json" -d '{"method":"CacheSrv.Get", "params":[{ "Key": "key3" }], "id": 0}' http://localhost:8080/rpc
```

To run unit and integration tests:

```
make test
```

This one will need to have Docker and go toolchain installed.

## Production/Scalability

The service can be brought to scale with each service instance acting as
separate shard of a distributed k/v store.

A second-level indexing service needs to be in place to direct requests to
appropriate shards. A good option would be a (replicated) hash service mapping
the entire key-space onto a set of httplru services using consistent hashing
scheme (optimal for adding/removing machines live).

Additionally each shard should be replicated and second-level service can decide
which replica to direct the request to based on one of load-balancing algorithms
(e.g. round-robin, least-loaded, etc).

Shards can be managed/deployed using simple Docker or Docker + one of cluster
management systems (e.g. Swam, Kubernetes, etc).

Monitoring instrumentation should be added to service shards to allow for
monitoring, alerting and effective load-balancing.

On the shard level, things that could be optimized are use of SSDs and perhaps
different database engine for better write performance.
