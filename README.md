# Distributed mutex using Lamport's Logical Clock

This is a naive implementation of distributed mutual exclusion using Lamport's Logical Clock<sup>1</sup>, done using Go and gRPC.

## Usage

To run a node:

```
$ go run . localhost:50051
```

Run a few nodes in separate terminals. Then for each node, connect to the other nodes using the command (one command per node):

```
> connect <addr>
```

Once each node is aware of the other nodes, to acquire the lock (it blocks until the lock is acquired):

```
> lock
```

To release the lock:

```
> unlock
```

## References

1. [Time, Clocks, and the Ordering of Events in a Distributed System, Leslie Lamport Massachusetts Computer Associates, Inc](https://lamport.azurewebsites.net/pubs/time-clocks.pdf)
