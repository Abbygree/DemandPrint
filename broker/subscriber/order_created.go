package subscriber

import (
	"context"
	"fmt"
	"github.com/korovkin/limiter"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"github.com/streadway/amqp"
	"gitlab.com/faemproject/backend/core/shared/lang"
	"gitlab.com/faemproject/backend/core/shared/logs"
	"gitlab.com/faemproject/backend/core/shared/structures/errpath"
	"gitlab.com/faemproject/backend/eda/eda.core/pkg/rabbit"
	"gitlab.com/faemproject/backend/eda/eda.core/services/orders/models"
	"gitlab.com/faemproject/backend/eda/eda.core/services/orders/proto"
	"math"
	"strings"
)

const (
	maxNewOrderStatesAllowed   = 100
	DemandCreatedOrderQueue    = "demand.order.created"
	DemandCreatedOrderConsumer = "demand.order.created"

	hmltTemplate = `Content-Type: text/html; charset="UTF-8"
Content-Transfer-Encoding: quoted-printable

<!DOCTYPE html><html>
	<head>
		<style>
   			h1 {
    		font-family: 'Times New Roman', Times, serif; 
    		font-size: 125%; 
			}
 		 </style>
	</head>
	<body>
		<h1>Text</h1>
	</body>
</html>`
)

func (s *Subscriber) HandleNewOrder(ctx context.Context, msg amqp.Delivery) error {
	var message, messageHTML string
	// Decode incoming message
	var order models.Order
	if err := s.Encoder.Decode(msg.Body, &order); err != nil {
		return errors.Wrap(err, "failed to decode order")
	}

	log := logs.Eloger.WithFields(logrus.Fields{
		"event":      "handling new order from rabbit",
		"order-uuid": order.UUID,
	})

	message += fmt.Sprintf("Заказ: %s<br>", order.ID)

	message += fmt.Sprintf("Сумма заказа: %d₽<br>Комментарий: %s<br><br>Состав заказа:<br>",
		int64(math.Ceil(order.CalcTotalPrice()*(1-order.Promotion.DiscountPercentage))),
		order.Comment)

	message += `<table border="1" width="100%" cellpadding="5">`
	for _, cartItem := range order.CartItems {
		message += `<tr>`
		message += fmt.Sprintf("<th>%s %d₽ x %d = %d₽</th>",
			cartItem.Product.Name,
			int64(cartItem.SingleItemPrice),
			cartItem.Count,
			int64(cartItem.TotalItemPrice))
		message += `<th style="width:5%;"></th>`
		message += `</tr>`
	}
	message += `</table>`

	messageHTML = strings.ReplaceAll(hmltTemplate, "Text", message)

	err := s.Handler.Gmail.SendMail([]string{"pomidor.orders@gmail.com"}, "Заказ №"+order.ID, messageHTML, false)
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
