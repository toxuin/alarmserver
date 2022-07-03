package webhooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/toxuin/alarmserver/config"
	"io/ioutil"
	"net/http"
	"strings"
)

type Bus struct {
	Debug    bool
	webhooks []config.WebhookConfig
	client   *http.Client
}

type WebhookPayload struct {
	Topic string `json:"topic"`
	Data  string `json:"data"`
}

func (webhooks *Bus) Initialize(conf config.WebhooksConfig) {
	fmt.Println("Initializing Webhook bus...")
	webhooks.client = &http.Client{}
	webhooks.webhooks = conf.Items
	// SET DEFAULT VALUES
	for index, item := range webhooks.webhooks {
		if item.Method == "" {
			item.Method = http.MethodPost
		}
		webhooks.webhooks[index] = item
	}

	for _, url := range conf.Urls {
		basicWebhook := config.WebhookConfig{
			Url:    url,
			Method: http.MethodPost,
		}
		webhooks.webhooks = append(webhooks.webhooks, basicWebhook)
	}
}

func (webhooks *Bus) SendMessage(topic string, data string) {
	for _, webhook := range webhooks.webhooks {
		payload := WebhookPayload{Topic: topic, Data: data}
		go webhooks.send(webhook, payload)
	}
}

func (webhooks *Bus) send(webhook config.WebhookConfig, payload WebhookPayload) {
	payloadJson, err := json.Marshal(payload)
	if err != nil {
		fmt.Println("WEBHOOKS: Error marshaling payload to JSON", err)
		return
	}

	request, err := http.NewRequest(webhook.Method, webhook.Url, bytes.NewBuffer(payloadJson))
	if err != nil {
		fmt.Printf("WEBHOOKS: Error creating %s request to %s\n", webhook.Method, webhook.Url)
		if webhooks.Debug {
			fmt.Println("Webhooks: Error", err)
		}
	}
	request.Header.Add("Content-Type", "application/json")
	if len(webhook.Headers) > 0 {
		for _, header := range webhook.Headers {
			headerParts := strings.Split(header, ": ")
			if len(headerParts) < 2 {
				continue
			}
			request.Header.Set(headerParts[0], headerParts[1])
		}
	}

	response, err := webhooks.client.Do(request)
	if err != nil {
		fmt.Printf("WEBHOOKS: Error delivering payload %s to %s\n", payloadJson, webhook.Url)
		if webhooks.Debug {
			fmt.Println("Webhooks: Error", err)
		}
	}
	if response == nil {
		fmt.Printf("WEBHOOKS: Got no response from %s\n", webhook.Url)
		return
	}
	if response.StatusCode != 200 {
		fmt.Printf(
			"WEBHOOKS: Got bad status code delivering payload to %s: %v\n",
			webhook.Url,
			response.StatusCode,
		)
	}
	if webhooks.Debug {
		bodyBytes, _ := ioutil.ReadAll(response.Body)
		bodyStr := string(bodyBytes)
		if len(bodyStr) == 0 {
			bodyStr = "*empty*"
		}
		fmt.Printf("WEBHOOKS: Response body: %s\n", bodyStr)
	}
}
