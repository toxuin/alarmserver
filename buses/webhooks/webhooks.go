package webhooks

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/toxuin/alarmserver/config"
	"io"
	"net/http"
	"strings"
	"text/template"
)

type Bus struct {
	Debug    bool
	webhooks []config.WebhookConfig
	client   *http.Client
}

type WebhookPayload struct {
	CameraName string `json:"cameraName"`
	EventType  string `json:"eventType"`
	Extra      string `json:"extra"`
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

func (webhooks *Bus) SendMessage(cameraName string, eventType string, extra string) {
	for _, webhook := range webhooks.webhooks {
		payload := WebhookPayload{CameraName: cameraName, EventType: eventType, Extra: extra}
		go webhooks.send(webhook, payload)
	}
}

func (webhooks *Bus) send(webhook config.WebhookConfig, payload WebhookPayload) {
	payloadJson, err := json.Marshal(payload)
	if err != nil {
		fmt.Println("WEBHOOKS: Error marshaling payload to JSON", err)
		return
	}

	var templateVars = map[string]interface{}{
		"Camera": payload.CameraName,
		"Event":  payload.EventType,
		"Extra":  payload.Extra,
	}

	// PARSE WEBHOOK URL AS TEMPLATE
	urlTemplate, err := template.New("webhookUrl").Parse(webhook.Url)
	if err != nil {
		fmt.Printf("WEBHOOKS: Error parsing webhook URL as template: %s\n", webhook.Url)
		if webhooks.Debug {
			fmt.Println("Webhooks: Error", err)
		}
		return
	}
	var urlBuffer bytes.Buffer
	err = urlTemplate.Execute(&urlBuffer, templateVars)
	if err != nil {
		fmt.Printf("WEBHOOKS: Error rendering webhook URL as template: %s\n", webhook.Url)
		if webhooks.Debug {
			fmt.Println("Webhooks: Error", err)
		}
		return
	}
	url := urlBuffer.String()

	// PARSE BODY AS TEMPLATE
	body := bytes.NewBuffer(payloadJson)
	if webhook.BodyTemplate != "" {
		bodyTemplate, err := template.New("payload").Parse(webhook.BodyTemplate)
		if err != nil {
			fmt.Printf("WEBHOOKS: Error parsing webhook body as template: %s\n", webhook.Url)
			if webhooks.Debug {
				fmt.Println("Webhooks: Error", err)
			}
			return
		}

		var bodyBuffer bytes.Buffer
		err = bodyTemplate.Execute(&bodyBuffer, templateVars)
		if err != nil {
			fmt.Printf("WEBHOOKS: Error rendering webhook body as template: %s\n", webhook.Url)
			if webhooks.Debug {
				fmt.Println("Webhooks: Error", err)
			}
			return
		}
		body = &bodyBuffer
	}

	request, err := http.NewRequest(webhook.Method, url, body)
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
		bodyBytes, _ := io.ReadAll(response.Body)
		bodyStr := string(bodyBytes)
		if len(bodyStr) == 0 {
			bodyStr = "*empty*"
		}
		fmt.Printf("WEBHOOKS: Response body: %s\n", bodyStr)
	}
}
