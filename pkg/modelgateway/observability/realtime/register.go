package realtime

import (
	"sync"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/modelgateway/observability/userrequest"
	bus "github.com/QuantumNous/new-api/pkg/realtime"
)

var (
	defaultTopic     *Topic
	defaultTopicOnce sync.Once
)

func RegisterDefaultTopic() {
	defaultTopicOnce.Do(func() {
		defaultTopic = NewTopic()
		bus.RegisterTopic(defaultTopic)
		model.AddModelExecutionObserver(func(record model.ModelExecutionRecord) {
			defaultTopic.Publish(record)
		})
		userrequest.AddObserver(func(event userrequest.Event) {
			defaultTopic.PublishUserRequest(event)
		})
	})
}
