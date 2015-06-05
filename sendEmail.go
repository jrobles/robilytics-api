package main

import (
	"encoding/base64"
	"fmt"
	"net/mail"
	"net/smtp"
	"strings"
)

func encodeRFC2047(String string) string {
	// use mail's rfc2047 to encode any string
	addr := mail.Address{String, ""}
	return strings.Trim(addr.String(), " <>")
}

func sendEmail(config *JSONConfigData, recipient string, body string, subject string) {

	smtpServer := "smtp.gmail.com"
	auth := smtp.PlainAuth(
		"",
		config.EmailAddress,
		config.EmailPassword,
		smtpServer,
	)

	header := make(map[string]string)
	header["Return-Path"] = "no-reply@robilytics.net"
	header["From"] = "no-reply@robilytics.net"
	header["To"] = recipient
	header["Subject"] = encodeRFC2047(subject)
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = "text/plain; charset=\"utf-8\""
	header["Content-Transfer-Encoding"] = "base64"

	message := ""
	for k, v := range header {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}
	message += "\r\n" + base64.StdEncoding.EncodeToString([]byte(body))

	// Connect to the server, authenticate, set the sender and recipient,
	// and send the email all in one step.
	err := smtp.SendMail(
		smtpServer+":587",
		auth,
		"no-reply@robilytics.net",
		[]string{recipient},
		[]byte(message),
	)
	if err != nil {
		fmt.Println(err)
	}
}
