package subscriber

import (
	"context"
	"fmt"
	"github.com/korovkin/limiter"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"github.com/zsefvlol/timezonemapper"
	"gitlab.com/faemproject/backend/core/shared/lang"
	"gitlab.com/faemproject/backend/core/shared/logs"
	"gitlab.com/faemproject/backend/core/shared/structures/errpath"
	"gitlab.com/faemproject/backend/eda/eda.core/pkg/rabbit"
	"gitlab.com/faemproject/backend/eda/eda.core/services/orders/models"
	"gitlab.com/faemproject/backend/eda/eda.core/services/orders/proto"
	"math"
	"time"
)

const (
	maxNewOrderStatesAllowed   = 100
	DemandCreatedOrderQueue    = "demand.order.created"
	DemandCreatedOrderConsumer = "demand.order.created"
)

func (s *Subscriber) HandleNewOrder(ctx context.Context, msg amqp.Delivery) error {
	var message string
	// Decode incoming message
	var order models.Order
	if err := s.Encoder.Decode(msg.Body, &order); err != nil {
		return errors.Wrap(err, "failed to decode order")
	}

	log := logs.Eloger.WithFields(logrus.Fields{
		"event":      "handling new order from rabbit",
		"order-uuid": order.UUID,
	})

	localTimezone := timezonemapper.LatLngToTimezoneString(float64(order.StoreData.Lat), float64(order.StoreData.Lon))
	log.WithFields(logrus.Fields{
		"lat":      order.StoreData.Lat,
		"lon":      order.StoreData.Lon,
		"timezone": localTimezone,
		"event":    "get store timezone",
	}).Print()
	loc, _ := time.LoadLocation(localTimezone)
	localTime := order.CreatedAt.In(loc)

	message += fmt.Sprintf("Заведение: %s (%s)\nЗаказ: %s (%s)\nКлиент: %s (%s)\n",
		order.StoreData.Name,
		order.StoreData.Address.UnrestrictedValue,

		order.ID,
		localTime.Format("2006-01-02 15:04:05"),

		order.ClientData.Name,
		order.ClientData.MainPhone)

	if order.WithoutDelivery == true {
		message += fmt.Sprintf("Доставка: %s\n", "Самовывоз")
	} else {
		message += fmt.Sprintf("Доставка: %s\n", order.DeliveryData.Address.UnrestrictedValue)
		if order.DeliveryData.AddressDetails.Entrance != "" {
			message += fmt.Sprintf("\t\tподъезд: %s,\n", order.DeliveryData.AddressDetails.Entrance)
		}
		if order.DeliveryData.AddressDetails.Apartment != "" {
			message += fmt.Sprintf("\t\tкв. %s,\n", order.DeliveryData.AddressDetails.Apartment)
		}
		if order.DeliveryData.AddressDetails.Intercom != "" {
			message += fmt.Sprintf("\t\tдомофон: %s,\n", order.DeliveryData.AddressDetails.Intercom)
		}
		if order.DeliveryData.AddressDetails.Floor != "" {
			message += fmt.Sprintf("\t\tэтаж: %s\n", order.DeliveryData.AddressDetails.Floor)
		}
	}

	paymentTypeMap := map[string]string{
		"cash": "наличными",
		"card": "картой",
	}

	message += fmt.Sprintf("Сумма заказа: %d₽(%s)\nКомментарий: %s\n\nСостав заказа:\n",
		int64(math.Ceil(order.CalcTotalPrice()*(1-order.Promotion.DiscountPercentage))),
		paymentTypeMap[order.PaymentType],
		order.Comment)

	for _, cartItem := range order.CartItems {
		message += fmt.Sprintf("- %s %d₽ x %d = %d₽\n",
			cartItem.Product.Name,
			int64(cartItem.SingleItemPrice),
			cartItem.Count,
			int64(cartItem.TotalItemPrice))
	}

	err := s.Handler.Gmail.SendMail([]string{"pomidor.orders@gmail.com"}, "Заказ №"+order.ID, message, false)
	if err != nil {
		log.WithError(err).Error("fail to send mail message")
	}

	return nil
}

func (s *Subscriber) initNewOrderState() error {
	receiverOrderStatesChannel, err := s.Rabbit.GetReceiver(rabbit.OrderStatesChannel)
	if err != nil {
		return errors.Wrapf(err, "failed to get a receiver channel")
	}

	// Declare an exchange first
	err = receiverOrderStatesChannel.ExchangeDeclare(
		rabbit.OrderExchange, // name
		"topic",              // type
		true,                 // durable
		false,                // auto-deleted
		false,                // internal
		false,                // no-wait
		nil,                  // arguments
	)
	if err != nil {
		return errors.Wrap(err, "failed to declare order state exchange")
	}

	// объявляем очередь для получения статусов заказов
	queue, err := receiverOrderStatesChannel.QueueDeclare(
		DemandCreatedOrderQueue, // name
		true,                    // durable
		false,                   // delete when unused
		false,                   // exclusive
		false,                   // no-wait
		nil,                     // arguments
	)
	if err != nil {
		return errors.Wrap(err, "failed to declare order state queue")
	}

	// биндим очередь для получения статусов заказов
	err = receiverOrderStatesChannel.QueueBind(
		queue.Name,                       // queue name
		"state."+proto.OrderStateCreated, // routing key
		rabbit.OrderExchange,             // exchange
		false,
		nil,
	)
	if err != nil {
		return errors.Wrap(err, "failed to bind order state queue")
	}

	msgs, err := receiverOrderStatesChannel.Consume(
		queue.Name,                 // queue
		DemandCreatedOrderConsumer, // consumer
		true,                       // auto-ack
		false,                      // exclusive
		false,                      // no-local
		false,                      // no-wait
		nil,                        // args
	)
	if err != nil {
		return errors.Wrap(err, "failed to consume from a channel")
	}

	s.wg.Add(1)
	go s.handleNewOrder(msgs) // handle incoming messages
	return nil
}

func (s *Subscriber) handleNewOrder(messages <-chan amqp.Delivery) {
	defer s.wg.Done()

	limit := limiter.NewConcurrencyLimiter(maxNewOrderStatesAllowed)
	defer limit.Wait()

	for {
		select {
		case <-s.closed:
			return
		case msg := <-messages:
			// Start new goroutine to handle multiple requests at the same time
			limit.Execute(lang.Recover(
				func() {
					if err := s.HandleNewOrder(context.Background(), msg); err != nil {
						logs.Eloger.Errorln(errpath.Err(err, "failed to handle new order state"))
					}
				},
			))
		}
	}
}
