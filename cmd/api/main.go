package main

import (
	"fmt"
	amqp "github.com/rabbitmq/amqp091-go"
	"log"
	"net/http"
	"os"
)

const webPort = "80"

type Config struct {
	Rabbit *amqp.Connection
}

func main() {
	log.Printf("Starting broker service on port  %s\n", webPort)

	rabbitConn, err := amqp.Dial("amqp://guest:guest@rabbitmq")
	if err != nil {
		log.Printf("failed initiating connection to RabbitMQ: %v", err)
		os.Exit(1)
	}

	defer rabbitConn.Close()
	log.Printf("Initiated connection to RabbitMQ")

	app := Config{
		Rabbit: rabbitConn,
	}

	srv := http.Server{
		Addr:    fmt.Sprintf(":%s", webPort),
		Handler: app.routes(),
	}

	err = srv.ListenAndServe()
	if err != nil {
		panic(err)
	}
}
