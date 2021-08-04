package subscriber

import (
	"go.uber.org/multierr"
	"sync"
	"time"

	"DemandPrint/broker"
	"DemandPrint/handler"
	"gitlab.com/faemproject/backend/core/shared/logs"
	"gitlab.com/faemproject/backend/core/shared/rabbit"
)

type Subscriber struct {
	Rabbit  *rabbit.Rabbit
	Encoder broker.Encoder
	Handler *handler.Handler

	wg     sync.WaitGroup
	closed chan struct{}
}

func (s *Subscriber) Init() error {
	s.closed = make(chan struct{})

	return multierr.Combine(
		s.initNewOrderState(),
	)
}

func (s *Subscriber) Wait(shutdownTimeout time.Duration) {
	// try to shutdown the listener gracefully
	stoppedGracefully := make(chan struct{}, 1)
	go func() {
		// Notify subscribers about exit, wait for their work to be finished
		close(s.closed)
		s.wg.Wait()
		stoppedGracefully <- struct{}{}
	}()

	// wait for a graceful shutdown and then stop forcibly
	select {
	case <-stoppedGracefully:
		logs.Eloger.Info("subscriber stopped gracefully")
	case <-time.After(shutdownTimeout):
		logs.Eloger.Info("subscriber stopped forcibly")
	}
}
