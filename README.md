This is an implementation of a binary memcached (bmemcached) server in Go.
It only supports get/set/delete.

## Run server:
* `go run bmcserver.go`

## Run tests:
* (Start up server)
* `python tests.py`
* (If you use the PyPI version of python-binary-memcached it will throw a string 
encoding error. The latest version of the library on GitHub has this fixed)

## Performance considerations:
* Access to the actual in-memory data store is mediated by a R/W mutex. This 
should allow safe concurrent access while providing optimal speed for reads 
if no writers hold the mutex.
* Only point of contention in the main routine is the `accept()` loop. When a 
new client connects a goroutine is set up to handle all further communication 
with that client and the main routine can continue accepting clients. Normally
you can specify a backlog argument to `listen()` but no such setting seems
available in the Go implementation. If a flood of connections happens all at
once some clients may not get accepted (and will probably retry).
* Timeouts for reading/writing on client sockets are _disabled_. This means that 
connections could pile up and starve the system of file descriptors. I believe 
this is preferable to disconnecting from long-running clients.
* The protocol has support for vbuckets allowing rehashing of data as the pool of 
servers grows/shrinks. This is handy if your application will catch on fire if you 
restart your memcached cluster. 
* This performs multiple `read()`s for each client request, first reading in the 
fixed header length and then parsing the variable-length pieces. I think the OS 
will buffer the packet data so the overhead shouldn't be horrible but reading 
in as much data at a time as is available and then parsing the big block of bytes
would certainly involve fewer context switches and buffer creations.

## bmemcached performance considerations:
* bmemcached appears to rely on packet ordering for the response packets. If only 
getk/setk/etc were used then response packet ordering would not matter and UDP might 
potentially be used. This would eliminate the overhead on connecting to a host and 
the danger of retries flooding the network in case of congestion. Retries are 
undesirable anyway.
* Jumbo frames might make things faster, especially for large keys/values.

## Spec
https://code.google.com/p/memcached/wiki/MemcacheBinaryProtocol
