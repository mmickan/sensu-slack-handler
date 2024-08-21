package main

import (
	"fmt"
	"github.com/sensu/sensu-plugin-sdk/sensu"
	"github.com/sensu/sensu-plugin-sdk/templates"
	corev2 "github.com/sensu/core/v2"
	"github.com/slack-go/slack"
	"os"
	"strings"
)

// HandlerConfig contains the Slack handler configuration
type HandlerConfig struct {
	sensu.PluginConfig
	sensuUIURL               string
	slackwebHookURL          string
	slackChannel             string
	slackUsername            string
	slackIconURL             string
	slackDescriptionTemplate string
	slackAlertCritical       bool
}

const (
	uiURL               = "ui-url"
	webHookURL          = "webhook-url"
	channel             = "channel"
	username            = "username"
	iconURL             = "icon-url"
	descriptionTemplate = "description-template"
	alertCritical       = "alert-on-critical"

	defaultChannel       = "#general"
	defaultIconURL       = "https://www.sensu.io/img/sensu-logo.png"
	defaultUsername      = "sensu"
	defaultTemplate      = `{{ if eq .Check.Status 0 }}:white_check_mark:{{ else if eq .Check.Occurrences 1 }}:warning:{{ else }}:repeat:{{ end }} *{{ if eq .Check.Status 0 }}OK{{ else if eq .Check.Status 1 }}WARNING{{ else if eq .Check.Status 2 }}CRITICAL{{ else }}UNKNOWN{{ end }}* *<{{ if index .Check.Annotations "runbook_url" }}{{ .Check.Annotations.runbook_url }}{{ else }}https://sensu.io{{ end }}|{{ .Check.Name }}>* on {{ .Entity.Name }}\n_{{ .Timestamp | UnixTime }}_\n{{ .Check.Output }}`
	defaultAlert    bool = false
)

var (
	config = HandlerConfig{
		PluginConfig: sensu.PluginConfig{
			Name:     "sensu-slack-handler",
			Short:    "The Sensu Go Slack handler for notifying a channel",
			Keyspace: "sensu.io/plugins/slack/config",
		},
	}

	slackConfigOptions = []sensu.ConfigOption{
		&sensu.PluginConfigOption[string]{
			Path:      uiURL,
			Env:       "SENSU_UI_URL",
			Argument:  uiURL,
			Shorthand: "s",
			Usage:     "The Sensu UI URL",
			Value:     &config.sensuUIURL,
		},
		&sensu.PluginConfigOption[string]{
			Path:      webHookURL,
			Env:       "SLACK_WEBHOOK_URL",
			Argument:  webHookURL,
			Shorthand: "w",
			Secret:    true,
			Usage:     "The webhook url to send messages to",
			Value:     &config.slackwebHookURL,
		},
		&sensu.PluginConfigOption[string]{
			Path:      channel,
			Env:       "SLACK_CHANNEL",
			Argument:  channel,
			Shorthand: "c",
			Default:   defaultChannel,
			Usage:     "The channel to post messages to",
			Value:     &config.slackChannel,
		},
		&sensu.PluginConfigOption[string]{
			Path:      username,
			Env:       "SLACK_USERNAME",
			Argument:  username,
			Shorthand: "u",
			Default:   defaultUsername,
			Usage:     "The username that messages will be sent as",
			Value:     &config.slackUsername,
		},
		&sensu.PluginConfigOption[string]{
			Path:      iconURL,
			Env:       "SLACK_ICON_URL",
			Argument:  iconURL,
			Shorthand: "i",
			Default:   defaultIconURL,
			Usage:     "A URL to an image to use as the user avatar",
			Value:     &config.slackIconURL,
		},
		&sensu.PluginConfigOption[string]{
			Path:      descriptionTemplate,
			Env:       "SLACK_DESCRIPTION_TEMPLATE",
			Argument:  descriptionTemplate,
			Shorthand: "t",
			Default:   defaultTemplate,
			Usage:     "The Slack notification output template, in Golang text/template format",
			Value:     &config.slackDescriptionTemplate,
		},
		&sensu.PluginConfigOption[bool]{
			Path:      alertCritical,
			Env:       "SLACK_ALERT_ON_CRITICAL",
			Argument:  alertCritical,
			Shorthand: "a",
			Default:   defaultAlert,
			Usage:     "The Slack notification will alert the channel with @channel",
			Value:     &config.slackAlertCritical,
		},
	}
)

func main() {
	goHandler := sensu.NewGoHandler(&config.PluginConfig, slackConfigOptions, checkArgs, sendMessage)
	goHandler.Execute()
}

func checkArgs(_ *corev2.Event) error {
	// Support deprecated environment variables
	if webhook := os.Getenv("SENSU_SLACK_WEBHOOK_URL"); webhook != "" {
		config.slackwebHookURL = webhook
	}
	if channel := os.Getenv("SENSU_SLACK_CHANNEL"); channel != "" && config.slackChannel == defaultChannel {
		config.slackChannel = channel
	}
	if username := os.Getenv("SENSU_SLACK_USERNAME"); username != "" && config.slackUsername == defaultUsername {
		config.slackUsername = username
	}
	if icon := os.Getenv("SENSU_SLACK_ICON_URL"); icon != "" && config.slackIconURL == defaultIconURL {
		config.slackIconURL = icon
	}

	if len(config.slackwebHookURL) == 0 {
		return fmt.Errorf("--%s or SLACK_WEBHOOK_URL environment variable is required", webHookURL)
	}

	if len(config.sensuUIURL) == 0 {
		return fmt.Errorf("--%s or SENSU_UI_URL environment variable is required", uiURL)
	}

	return nil
}

func formattedEventAction(event *corev2.Event) string {
	switch event.Check.Status {
	case 0:
		return "RESOLVED"
	default:
		return "ALERT"
	}
}

func chomp(s string) string {
	return strings.Trim(strings.Trim(strings.Trim(s, "\n"), "\r"), "\r\n")
}

func eventKey(event *corev2.Event) string {
	return fmt.Sprintf("%s/%s", event.Entity.Name, event.Check.Name)
}

func eventSummary(event *corev2.Event, maxLength int) string {
	output := chomp(event.Check.Output)
	if len(event.Check.Output) > maxLength {
		output = output[0:maxLength] + "..."
	}
	return fmt.Sprintf("%s:%s", eventKey(event), output)
}

func eventURL(event *corev2.Event) string {
	return fmt.Sprintf("%s/n/%s/events/%s/%s", config.sensuUIURL, event.Entity.Namespace, event.Entity.Name, event.Check.Name)
}

func formattedMessage(event *corev2.Event) string {
	return fmt.Sprintf("%s - %s", formattedEventAction(event), eventSummary(event, 100))
}

func messageColor(event *corev2.Event) string {
	switch event.Check.Status {
	case 0:
		return "#36a64f"
	case 1:
		return "#ffcc00"
	case 2:
		return "#ff0000"
	default:
		return "#6600cc"
	}
}

func messageAttachment(event *corev2.Event) slack.Attachment {
	description, err := templates.EvalTemplate("description", config.slackDescriptionTemplate, event)
	if err != nil {
		fmt.Printf("%s: Error processing template: %s", config.PluginConfig.Name, err)
	}

	description = strings.Replace(description, `\n`, "\n", -1)
	attachment := slack.Attachment{
		Text:       description,
		Fallback:   formattedMessage(event),
		Color:      messageColor(event),
		MarkdownIn: []string{
			"text",
		},
		Actions:    []slack.AttachmentAction{
			{
				Text:  "View in Sensu",
				Type:  "button",
				URL:   eventURL(event),
			},
		},
	}
	return attachment
}

func sendMessage(event *corev2.Event) error {
	hookmsg := &slack.WebhookMessage{
		Attachments: []slack.Attachment{messageAttachment(event)},
		Channel:     config.slackChannel,
		IconURL:     config.slackIconURL,
		Username:    config.slackUsername,
	}

	err := slack.PostWebhook(config.slackwebHookURL, hookmsg)
	if err != nil {
		return fmt.Errorf("Failed to send Slack message: %v", err)
	}

	// FUTURE: send to AH
	fmt.Printf("Notification sent to Slack channel %s\n", config.slackChannel)

	return nil
}
