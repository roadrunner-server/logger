# Docs: [link](https://docs.roadrunner.dev/logging-and-observability/logger)

## Asynchronous output

All enabled logger modes use asynchronous output. Each configured logger has one output goroutine and a queue limited to 1 MiB of formatted data. An additional batch of up to 1 MiB can be in progress. Log calls format and copy messages before they return.

When the queue has insufficient space, the logger discards the new message. Messages larger than 1 MiB are also discarded. Shutdown reports the discard count and output errors.

Shutdown rejects new messages and drains accepted messages before it closes output files. It waits for up to five seconds, or until the shutdown context expires. On timeout, it discards queued data and returns an error. An output write that has already started can remain blocked until the output becomes available. An abrupt process exit can lose queued messages.
