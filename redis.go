package main

import (
	"github.com/garyburd/redigo/redis"
)

func connectToRedis(address string) redis.Conn {
	redisConn, err := redis.Dial("tcp", address)
	if err != nil {
		errorToLog(errorLogFile, "Could not connect to Redis server", err)
	}
	return redisConn
}
