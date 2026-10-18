# httpserver

`Run` starts the server in a goroutine, waits for context cancellation or a server error, then calls `Shutdown` with a timeout so in-flight requests finish before the pod stops (graceful shutdown).
