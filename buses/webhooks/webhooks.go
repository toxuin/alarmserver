package webhooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/toxuin/alarmserver/config"
	"net/http"
)

type Bus struct {
	Debug  bool
	urls   []string
	client *http.Client
}

type WebhookPayload struct {
	Topic string `json:"topic"`
	Data  string `json:"data"`
}

func (webhooks *Bus) Initialize(config config.WebhooksConfig) {
	fmt.Println("Initializing Webhook bus...")
	webhooks.client = &http.Client{}
	webhooks.urls = config.Urls
}

func (webhooks *Bus) SendMessage(topic string, data string) {
	for _, url := range webhooks.urls {
		payload := WebhookPayload{Topic: topic, Data: data}
		go webhooks.send(url, payload)
	}
}

func (webhooks *Bus) send(url string, payload WebhookPayload) {
	payloadJson, err := json.Marshal(payload)
	if err != nil {
		fmt.Println("WEBHOOKS: Error marshaling payload to JSON", err)
		return
	}
	response, err := webhooks.client.Post(url, "application/json", bytes.NewBuffer(payloadJson))
	if err != nil {
		fmt.Printf("WEBHOOKS: Error delivering payload %s to %s\n", payloadJson, url)
	}
	if response == nil {
		fmt.Printf("WEBHOOKS: Got no response from %s", url)
	}
	if response != nil && response.StatusCode != 200 {
		fmt.Printf("WEBHOOKS: Got bad status code delivering payload to %s: %v\n", url, response.StatusCode)
	}
}
