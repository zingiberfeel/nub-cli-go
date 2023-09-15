package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/urfave/cli/v2"
)

type RabbitConfig struct {
	Username string
	Password string
	Host     string
	Port     string
}

type SnippetMessage struct {
	Text      string    `json:"text"`
	Lifetime  time.Time `json:"lifetime"`
	Extension string    `json:"extension"`
}

func failOnError(err error, msg string) {
	if err != nil {
			log.Panicf("%s: %s", msg, err)
	}
}

func assembleSnippet(filePath string, days int, hours int, minutes int) SnippetMessage {
			lastDotIndex := strings.LastIndex(filePath, ".")
			extension := filePath[lastDotIndex+1:]

			file, err := os.Open(filePath)
			failOnError(err, "Failed to open a file")

			fileContent, err := io.ReadAll(file)
			failOnError(err, "Failed to read a file")
			
			currentDateTime := time.Now();

			daysInMinutes := time.Duration(days) * 24 * time.Hour
            hoursInMinutes := time.Duration(hours) * time.Hour
            minutesDuration := time.Duration(minutes) * time.Minute
            totalMinutes := daysInMinutes + hoursInMinutes + minutesDuration

            lifetime := currentDateTime.Add(totalMinutes)
		
			snippetMessage := SnippetMessage{
                Text:     string(fileContent),
                Lifetime: lifetime,
				Extension: extension,
            }

			return snippetMessage
}

func randomString(l int) string {
	bytes := make([]byte, l)
	for i := 0; i < l; i++ {
			bytes[i] = byte(randInt(65, 90))
	}
	return string(bytes)
}

func randInt(min int, max int) int {
	return min + rand.Intn(max-min)
}

func getEnv(key, defaultValue string) string {
	value, exists := os.LookupEnv(key)
	if !exists {
		return defaultValue
	}
	return value
}

func loadConfig() RabbitConfig {
	config := RabbitConfig{
		Username: getEnv("RABBITMQ_USERNAME", "guest"),
		Password: getEnv("RABBITMQ_PASSWORD", "guest"),
		Host:     getEnv("RABBITMQ_HOST", "localhost"),
		Port:     getEnv("RABBITMQ_PORT", "5672"),
	}

	return config
}

func startRabbitRPC(snippet SnippetMessage) (res string) {
	config := loadConfig()
	connectionString := fmt.Sprintf("amqp://%s:%s@%s:%s/", config.Username, config.Password, config.Host, config.Port)
	conn, err := amqp.Dial(connectionString)
			failOnError(err, "Failed to connect to RabbitMQ")

			ch, err := conn.Channel()
			failOnError(err, "Failed to open a channel")
			defer ch.Close()

			q, err := ch.QueueDeclare(
                "",    // name
                false, // durable
                false, // delete when unused
                true,  // exclusive
                false, // noWait
                nil,   // arguments
        	)

			failOnError(err, "Failed to declare a queue")

			msgs, err := ch.Consume(
                q.Name, // queue
                "",     // consumer
                true,   // auto-ack
                false,  // exclusive
                false,  // no-local
                false,  // no-wait
                nil,    // args
        	)
        	failOnError(err, "Failed to register a consumer")

			corrId := randomString(32)

			context, cancel := context.WithTimeout(context.Background(), 5*time.Second)
        	defer cancel()

			body, err := json.Marshal(snippet)
			failOnError(err, "Failed to serialize a snippet")
			
			err = ch.PublishWithContext(context,
                "",          // exchange
                "rpc_queue", // routing key
                false,       // mandatory
                false,       // immediate
                amqp.Publishing{
                        ContentType:   "application/json",
                        CorrelationId: corrId,
                        ReplyTo:       q.Name,
                        Body:          body,
                })
        	failOnError(err, "Failed to publish a message")

			for d := range msgs {
				if corrId == d.CorrelationId {
					res = string(d.Body)
					break
				}
			}
			return 
}

func main() {
	log.SetOutput(os.Stdout)
	log.SetPrefix("[NubCLI] ")
	log.SetFlags(log.Ldate | log.Ltime)

	app := &cli.App{
		Name: "nub",
		Version: "1.0",
		Usage: "Create a snippet of file lines with a specified lifetime",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name: "file",
				Usage: "Path to the file",
			},
			&cli.IntFlag{
				Name: "days",
				Usage: "Number of days",
				Value: 0,
			},
			&cli.IntFlag{
				Name: "hours",
				Usage: "Number of hours",
				Value: 0,
			},
			&cli.IntFlag{
                Name:  "minutes",
                Usage: "Number of minutes",
                Value: 0,
            },
		},
		Action: func(ctx *cli.Context) error {
			filePath := ctx.String("file")
			days := ctx.Int("days")
			hours := ctx.Int("hours")
			minutes := ctx.Int("minutes")

			snippet := assembleSnippet(filePath, days, hours, minutes)
			
			
			log.Printf(" [x] Requesting a snippet link")

			res := startRabbitRPC(snippet)
			
			log.Printf(" [.] Here you go: \x1b]8;;%s\x1b\\%s\x1b]8;;\x1b\\", res, res)
			
            return nil
		},
	}
	err := app.Run(os.Args)
	if err != nil {
		fmt.Println("Error: ", err)
		os.Exit(1)
	}

}