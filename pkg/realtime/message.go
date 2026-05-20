package realtime

const (
	MessageTypeSubscribe   = "subscribe"
	MessageTypeUnsubscribe = "unsubscribe"
	MessageTypePing        = "ping"
	MessageTypePong        = "pong"
	MessageTypeSnapshot    = "snapshot"
	MessageTypeDelta       = "delta"
	MessageTypeStatus      = "status"
	MessageTypeError       = "error"
)

type ClientMessage struct {
	Type   string         `json:"type"`
	ID     string         `json:"id,omitempty"`
	Topic  string         `json:"topic,omitempty"`
	Params map[string]any `json:"params,omitempty"`
}

type ServerMessage struct {
	Type     string `json:"type"`
	ID       string `json:"id,omitempty"`
	Topic    string `json:"topic,omitempty"`
	Sequence int64  `json:"sequence,omitempty"`
	SentAt   int64  `json:"sent_at,omitempty"`
	Data     any    `json:"data,omitempty"`
	Message  string `json:"message,omitempty"`
}

type Subscription struct {
	ID     string
	Topic  string
	Params map[string]any
}

type Subscriber interface {
	Send(message ServerMessage) bool
}

type Topic interface {
	Name() string
	Subscribe(subscriber Subscriber, subscription Subscription)
	Unsubscribe(subscriber Subscriber, subscriptionID string)
}
