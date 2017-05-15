// Copyright 2017 Vector Creations Ltd
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package matrus provides a matrix.org hook for logrus logging
package matrus

import (
	"errors"
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/matrix-org/gomatrix"
)

const (
	// defaultBatchPeriod defines the default interval at which messages should be
	// ispatched to the matrix.org logging room
	defaultBatchPeriod = 15   // seconds
	maxQueuedMessages  = 1000 // Max number of logging messages to buffer
)

// MHook is a matrus Hook for logging messages to the specified matrix.org room
// MHook implements logrus.Hook interface
type MHook struct {
	AcceptedLevels  []logrus.Level
	Client          *gomatrix.Client
	LoggingRoomID   string
	formatter       logrus.Formatter
	batchedMessages []string
	batchTicker     *time.Ticker
}

// New instance of matrus logger hook
//  * "cli" - Gomatrix client instance
//  * "loggingRoomID" - The matrix.org roomID to send logging events to
//  * "level" - Events at this logging level or higher will be dispatched
//  * "bp" - The interval in seconds at which batches of logging events will be dispatched to matrix.org
//  (if < 1 the batch dispatch period is set to the default of 15s)
func New(cli *gomatrix.Client, loggingRoomID string, level logrus.Level, bp int) (*MHook, error) {
	if cli == nil {
		return nil, errors.New("Invalid gomatrix client")
	} else if loggingRoomID == "" {
		return nil, errors.New("Invalid matrix.org room ID")
	}

	// Set the batch dispatcher period
	if bp < 1 {
		bp = defaultBatchPeriod
	}

	hook := MHook{
		Client:         cli,
		LoggingRoomID:  loggingRoomID,
		AcceptedLevels: logLevelsFrom(level),
		formatter:      &matrixFormatter{},
		batchTicker:    time.NewTicker(time.Second * time.Duration(bp)),
	}

	// Start periodic dispatcher
	go func() {
		for range hook.batchTicker.C {
			hook.sendBatchedMessages()
		}
	}()

	return &hook, nil
}

// Levels gets the levels at which logging events should be sent to matrix.org
func (matrusHook *MHook) Levels() []logrus.Level {
	if matrusHook.AcceptedLevels == nil {
		return allLevels
	}
	return matrusHook.AcceptedLevels
}

// Fire queues messages to be dispatched to the matrix.org logging room
func (matrusHook *MHook) Fire(e *logrus.Entry) error {
	htmlbytes, err := matrusHook.formatter.Format(e)
	html := string(htmlbytes)
	if err != nil || html == "" {
		return nil
	}

	// Append new message
	matrusHook.batchedMessages = append(matrusHook.batchedMessages, html)
	// Truncate messages if larger than maxQueuedMessages
	if len(matrusHook.batchedMessages) > maxQueuedMessages {
		matrusHook.batchedMessages = matrusHook.batchedMessages[(len(matrusHook.batchedMessages) - maxQueuedMessages):]
	}
	return nil
}

// sendBatchedMessages periodically dispatches messages in to the matrix.org logging room
func (matrusHook *MHook) sendBatchedMessages() (bool, error) {
	if len(matrusHook.batchedMessages) > 0 {
		if err := matrusHook._HTMLMessage("m.text",
			strings.Join(matrusHook.batchedMessages, "<br/>"),
			strings.Join(matrusHook.batchedMessages, "\n")); err == nil {
			matrusHook.batchedMessages = make([]string, 0)
		}
		return true, err
	}
	return false, nil
}

// _HTMLMessage sends an HTML formatted message in to the matrix.org logging room
func (matrusHook *MHook) _HTMLMessage(msgType, html, body string) error {
	msg := gomatrix.GetHTMLMessage(msgType, html)
	if body != "" {
		msg.Body = body
	}
	_, err := matrusHook.Client.SendMessageEvent(matrusHook.LoggingRoomID, "m.room.message",
		msg)
	return err
}

// matrixFormatter message formatter
type matrixFormatter struct{}

// Format formats a message to send to matrix
func (formatter *matrixFormatter) Format(e *logrus.Entry) ([]byte, error) {
	var color string

	switch e.Level {
	case logrus.WarnLevel:
		color = "orange"
	case logrus.InfoLevel:
		color = "green"
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		color = "red"
	default:
		color = "lightblue"
	}

	htmlMsg := fmt.Sprintf(`<font color="%s">`, color)

	var data string
	for k, v := range e.Data {
		if k != "msg" {
			data += k + "=" + fmt.Sprint(v) + ", "
		}
	}

	data = strings.TrimSuffix(data, ", ")
	data = html.EscapeString(data)

	msgBody := strings.TrimSpace(e.Message)
	msgBody = html.EscapeString(msgBody)

	if data == "" && msgBody == "" {
		return nil, errors.New("Empty logging event")
	}

	if data != "" {
		htmlMsg += "[" + data + "] - "
	}

	if msgBody != "" {
		htmlMsg += fmt.Sprintf(`<b>%s</b>`, msgBody)
	}

	htmlMsg += `</font>`
	return []byte(htmlMsg), nil
}
