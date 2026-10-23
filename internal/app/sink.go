package app

// Sinks publishes every status to each of its sinks, in order.
type Sinks []Sink

// Publish implements Sink.
func (s Sinks) Publish(st Status) {
	for _, sink := range s {
		sink.Publish(st)
	}
}
