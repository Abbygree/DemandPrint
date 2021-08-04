module DemandPrint

go 1.16

require (
	github.com/korovkin/limiter v0.0.0-20190919045942-dac5a6b2a536
	github.com/njern/gogmail v0.0.0-20140420091239-af23893c9766
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.8.1
	github.com/spf13/viper v1.8.1
	github.com/streadway/amqp v1.0.0
	gitlab.com/faemproject/backend/core/shared v0.9.24
	gitlab.com/faemproject/backend/eda/eda.core v0.29.2
	go.uber.org/multierr v1.7.0
)

replace (
	//gitlab.com/faemproject/backend/core/shared => ../shared
	gitlab.com/faemproject/backend/core/shared => gitlab.com/faemproject/backend/core/shared.git v0.9.24
	gitlab.com/faemproject/backend/eda/eda.core => gitlab.com/faemproject/backend/eda/eda.core.git v0.29.2
)
