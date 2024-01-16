package event

import (
	"bytes"
	"encoding/json"
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"log"
	"net/http"
)

type Consumer struct {
	conn      *amqp.Connection
	queueName string
}

type Payload struct {
	Name string `json:"name"`
	Data string `json:"data"`
}

func NewConsumer(conn *amqp.Connection) (Consumer, error) {
	consumer := Consumer{
		conn: conn,
	}

	err := consumer.setup()
	return consumer, err
}

func (c *Consumer) setup() error {
	channel, err := c.conn.Channel()
	if err != nil {
		return err
	}

	return declareExchange(channel)
}

func (c *Consumer) Listen(topics []string) error {
	ch, err := c.conn.Channel()
	if err != nil {
		return err
	}
	defer ch.Close()

	queue, err := declareRandomQueue(ch)
	if err != nil {
		return err
	}

	for _, topic := range topics {
		err := ch.QueueBind(queue.Name, topic, "logs_topic", false, nil)
		if err != nil {
			return err
		}
	}

	msgs, err := ch.Consume(queue.Name, "", true, false, false, false, nil)
	if err != nil {
		return err
	}

	forever := make(chan bool)
	go func() {
		for d := range msgs {
			var payload Payload
			json.Unmarshal(d.Body, &payload)

			go handlePayload(payload)
		}
	}()

	fmt.Printf("Waiting for message on [Exchange, Queue] [logs_topic, %s]", queue.Name)
	<-forever

	return nil
}

func handlePayload(payload Payload) {
	switch payload.Name {
	case "log", "event":
		err := logEvent(payload)
		if err != nil {
			log.Println(err)
		}
	case "auth":
	//authenticate
	default:
		err := logEvent(payload)
		if err != nil {
			log.Println(err)
		}

	}
}

func logEvent(payload Payload) error {
	jsonData, _ := json.Marshal(payload)
	request, err := http.NewRequest("POST", "http://logger-service", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}

	httpClient := &http.Client{}

	rsp, err := httpClient.Do(request)
	if err != nil {
		return err
	}

	if rsp.StatusCode != http.StatusAccepted {
		fmt.Printf("response code: %d \n", rsp.StatusCode)
		return err
	}

	defer rsp.Body.Close()

	return nil
}
