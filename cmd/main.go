package main

import (
	"DemandPrint/broker/publisher"
	"DemandPrint/broker/subscriber"

	/*"DemandPrint/broker/subscriber"*/
	"DemandPrint/config"
	"DemandPrint/handler"
	"fmt"
	"github.com/njern/gogmail"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"gitlab.com/faemproject/backend/core/shared/rabbit"
	"io"
	"os"
	"time"

	oss "gitlab.com/faemproject/backend/core/shared/os"
)

const (
	defaultConfigPath     = "exporter.toml"
	brokerShutdownTimeout = 30 * time.Second
)

func main() {
	logger := logrus.WithFields(logrus.Fields{
		"event": "init project",
	})

	cfg, err := config.Parse(defaultConfigPath)
	if err != nil {
		logger.Fatalf("failed to parse the config file: %v", err)
	}

	err = initLogrus(cfg)
	if err != nil {
		logger.Fatalf("failed to parse the config file: %v", err)
	}

	//fmt.Printf("%+v\n", cfg)

	hdlr := &handler.Handler{}

	hdlr.Config = cfg

	// Connect to the broker and remember to close it
	rmq := &rabbit.Rabbit{
		Credits: rabbit.ConnCredits{
			URL:  cfg.Broker.UserURL,
			User: cfg.Broker.UserCredits,
		},
	}
	if err = rmq.Init(cfg.Broker.ExchangePrefix, cfg.Broker.ExchangePostfix); err != nil {
		logger.Fatalf("failed to connect to RabbitMQ: %v", err)
	}
	defer rmq.CloseRabbit()

	// Create a publisher
	pub := publisher.Publisher{
		Rabbit:  rmq,
		Encoder: &rabbit.JsonEncoder{},
	}
	if err = pub.Init(); err != nil {
		logger.Fatalf("failed to init the publisher: %v", err)
	}
	defer pub.Wait(brokerShutdownTimeout)

	hdlr.Pub = &pub
	hdlr.Gmail = gogmail.GmailConnection("sanakoev.alibek@gmail.com", "Alibek99")

	// Create a subscriber
	sub := subscriber.Subscriber{
		Rabbit:  rmq,
		Encoder: &rabbit.JsonEncoder{},
		Handler: hdlr,
	}
	if err = sub.Init(); err != nil {
		logger.Fatalf("failed to start the subscriber: %v", err)
	}
	defer sub.Wait(brokerShutdownTimeout)

	// Wait for program exit
	<-oss.NotifyAboutExit()
}

func initLogrus(cfg *config.Config) error {
	lvl, err := logrus.ParseLevel(cfg.Application.LogLevel)
	if err != nil {
		return errors.Wrap(err, "failed to parse a log level")
	}
	logrus.SetLevel(lvl)

	logrus.SetFormatter(&logrus.JSONFormatter{})

	const dir = "logs"
	err = os.MkdirAll(dir, os.ModePerm)
	if err != nil {
		return err
	}

	now := time.Now()
	logFileName := fmt.Sprintf("%s/%s.log", dir, now.Format("2006-01-02T15-04-05"))
	logFile, err := os.OpenFile(logFileName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return err
	}

	mw := io.MultiWriter(os.Stdout, logFile)
	logrus.SetOutput(mw)

	return nil
}
