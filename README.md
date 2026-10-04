# Docs: [link](https://docs.roadrunner.dev/logging-and-observability/logger)

## Asynchronous output

All enabled logger modes use asynchronous output. Each configured logger has one output goroutine and a buffered channel for up to 1,024 records. Memory use depends on record size. Log calls format and copy messages before they return.

When the channel is full, the logger discards the new message. Shutdown reports output errors.

Shutdown rejects new messages and drains accepted messages before it closes output files. It waits for up to five seconds, or until the shutdown context expires. On timeout, it discards queued data and returns an error. An output write that has already started can remain blocked until the output becomes available. An abrupt process exit can lose queued messages.
