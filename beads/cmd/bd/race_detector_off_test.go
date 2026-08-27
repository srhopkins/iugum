//go:build !race

package bdcmd

// raceEnabled is false in non-race builds; see race_detector_on_test.go.
const raceEnabled = false
