package embedbin

// Silverbullet is the embedded wiki binary. Root package main sets it via Set.
var Silverbullet []byte

func Set(b []byte) { Silverbullet = b }
