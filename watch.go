package runit

// WatchEvent represents a status change event from watching a service
type WatchEvent struct {
	Status Status
	Err    error
}
