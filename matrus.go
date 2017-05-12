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
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/matrix-org/gomatrix"
)

const (
	// defaultBatchPeriod defines the default interval at which messages should be
	// ispatched to the matrix.org logging room
	defaultBatchPeriod = 15 // seconds
)

// MHook is a matrus Hook for logging messages to the specified matrix.org room
// MHook implements logrus.Hook interface
type MHook struct {
	AcceptedLevels  []logrus.Level
	Client          *gomatrix.Client
	LoggingRoomID   string
	Asynchronous    bool
	formatter       logrus.Formatter
	batchedMessages string
	batchTicker     *time.Ticker
}

// New instance of matrus logger hook
func New(cli *gomatrix.Client, loggingRoomID string, level logrus.Level, bp int) (*MHook, error) {
	if cli == nil {
		return nil, errors.New("Invalid gomatrix client")
	} else if loggingRoomID == "" {
		return nil, errors.New("Invalid matrix.org room ID")
	}

	// Set the batch dispatcher period
	if bp == 0 {
		bp = defaultBatchPeriod
	}

	hook := MHook{
		Client:         cli,
		LoggingRoomID:  loggingRoomID,
		Asynchronous:   false,
		AcceptedLevels: logLevelsFrom(level),
		formatter:      &MatrixFormatter{},
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

// Levels sets which levels to log to the matrix.org logging room
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

	if matrusHook.batchedMessages != "" {
		matrusHook.batchedMessages += "<br/>"
	}

	matrusHook.batchedMessages += html
	return nil
}

// sendBatchedMessages periodically dispatches messages in to the matrix.org logging room
func (matrusHook *MHook) sendBatchedMessages() (bool, error) {
	if matrusHook.batchedMessages != "" {
		err := matrusHook._HTMLMessage("m.text", matrusHook.batchedMessages, "")
		if err == nil {
			matrusHook.batchedMessages = ""
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

// MatrixFormatter message formatter
type MatrixFormatter struct{}

// Format formats a message to send to matrix
func (formatter *MatrixFormatter) Format(e *logrus.Entry) ([]byte, error) {

	if msg, err := e.String(); msg == "" || err != nil || e.Message == "" {
		return nil, errors.New("Empty logging event")
	}

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

	html := `<font color=` + color + `>`

	data := ""
	for k, v := range e.Data {
		if k != "msg" {
			data += k + "=" + fmt.Sprint(v) + ", "
		}
	}
	strings.TrimSuffix(data, ", ")

	htmlBody := e.Message
	htmlBody = strings.Replace(htmlBody, "<", "&lt;", -1)
	htmlBody = strings.Replace(htmlBody, ">", "&gt;", -1)

	if data != "" {
		html += "[" + data + "] - "
	}
	html += htmlBody

	html += `</font>`
	return []byte(html), nil
}
