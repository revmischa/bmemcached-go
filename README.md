This is an implementation of a binary memcached (bmemcached) server in Go.
It only supports get/set/delete.

## Run server:
* `go run bmcserver.go`

## Run tests:
* (Start up server)
* `python tests.py`

## Performance considerations:
* This passes []byte arrays around, which I believe is an expensive copy. There is probably a more efficient way to pass them by reference.
* Access to the actual in-memory data store is mediated by a R/W mutex. This should allow safe concurrent access while providing optimal speed for reads if no writers hold the mutex.
* Only point of contention in the main routine is the `accept()` loop. When a new client connects a goroutine is set up to handle all further communication with that client and the main routine can continue accepting clients.
* Timeouts for reading/writing on client sockets is _disabled_. This means that connections could pile up and starve the system of file descriptors. I believe this is preferable to disconnecting from long-running clients.
* The protocol has support for vbuckets allowing rehashing of data as the pool of servers grows/shrinks. This is handy if your application will catch on fire if you restart your memcached cluster. 

bmemcached performance considerations:
* bmemcached appears to rely on packet ordering for the response packets. If only getk/setk/etc were used then response packet ordering would not matter and UDP might potentially be used. This would eliminate the overhead on connecting to a host and the danger of retries flooding the network in case of congestion. Retries are undesirable anyway.
* Jumbo frames might make things faster, especially for large keys/values.

## Spec
https://code.google.com/p/memcached/wiki/MemcacheBinaryProtocol
