//go:build integration && race

package test

// raced says the race detector watches the run, which makes timings meaningless.
const raced = true
