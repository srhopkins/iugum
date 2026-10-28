package main

import "github.com/srhopkins/iugum/agentcore"

// The server owns this opaque payload; a browser can only hold its random ID.
type nativePreparedTurn struct {
	decision *agentcore.PreparedTurn
	snapshot string
}
