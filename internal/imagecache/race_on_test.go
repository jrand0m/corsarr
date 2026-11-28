//go:build race

package imagecache

// raceOn shortens heavy load tests under the race detector.
const raceOn = true
