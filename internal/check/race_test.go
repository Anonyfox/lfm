//go:build race

package check

// The race detector slows analysis several times over.
const slowdown = 5
