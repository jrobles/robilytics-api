package main

import (
	"fmt"
	"log"
	"os"
)

func errorToLog(logFile string, logData string, err error) {
	f, err := os.OpenFile(logFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		fmt.Println("ERROR: Cannot write to log file")
		panic(err)
	}
	defer f.Close()
	log.SetOutput(f)
	log.Println(logData, err)
}
