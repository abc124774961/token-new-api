package realtime

var defaultHub = NewHub()

func DefaultHub() *Hub {
	return defaultHub
}

func RegisterTopic(topic Topic) {
	defaultHub.Register(topic)
}
