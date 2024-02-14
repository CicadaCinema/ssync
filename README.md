## `ssync` stands for "Simperium Sync".

This package piggybacks on top of the Simperium protocol to provide an easy-to-use experience for syncing arbitrary data.

See the two example applications for details (they both function well using the Simperium instance provided by Simplenote, at the time of writing).

In short, to implement syncing on top of this package, you must:
- Create some struct to store the state of your application, let's call it `Bucket`.
- Ensure that `Bucket` satisfies the `InitAndUpdate` interface by implementing two methods, `Init` and `Update`.
- The `Init` method runs exactly once, before `ssync.StartSyncing` returns, and populates the instance of `Bucket` with the remote state.
- The `Update` method runs in a separate goroutine whenever some state is received from the server after syncing has started.
- If you wish to respond to local events, send values to a `chan ssync.Change` channel to save them into the application state. Note that `Update` will also run in response to local changes (see `ignoreNextUpdate` in demo 2 for an example of this).

Effectively all that's needed is to implement two methods and launch one goroutine to monitor for local behaviour and update the server accordingly - the last three bullet points above.

One nice property of the client-server model is that two clients do not have to be online at the same time for a change to propagate between them.

Missing features in the demos:
- Any kind of error handling.
- Dealing with race conditions.
- Saving state locally. (On startup, all the state is fetched afresh from the server, and all the local state is destroyed on shutdown.)

Possible applications for this package:
- An alternative to Syncthing that takes advantage of the benefits of the client-server model by not requiring clients to be online at the same time to propagate changes.
- An even higher-level library that performs a function such as "given the same set of secrets, magically syncs two structs across an arbitrary number of Go programs running concurrently". This is a cool application of this technology in my opinion. Perhaps it can be used to implement multiplayer board games in an easy way. The struct may need a mutex, but other than that, any fields can just be serialised to JSON and synced over `ssync`.

Resources:
- https://pkg.go.dev/github.com/gorilla/websocket
- https://github.com/gorilla/websocket/tree/main/examples/echo
- https://github.com/Simperium/simperium-protocol/blob/master/SYNCING.md (This is the spec for the Simperium protocol.)
- https://github.com/apokalyptik/go-jsondiff (Not used here because I only needed coarse grained functionality.)
- https://github.com/simperium/jsondiff
- https://simperium.github.io/jsondiff/jsondiff-js.html
- https://pkg.go.dev/github.com/sergi/go-diff/diffmatchpatch
- https://github.com/fsnotify/fsnotify
