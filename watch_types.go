package runit

// WatchCleanupFunc is a function that cleans up watch resources
type WatchCleanupFunc func() error