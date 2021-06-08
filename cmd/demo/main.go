package main

import (
	"io"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/blakewilliams/viewproxy"
)

func main() {
	target := getTarget()
	server := viewproxy.NewServer(target)
	server.Port = getPort()
	server.ProxyTimeout = time.Duration(5) * time.Second
	server.Logger = buildLogger()
	server.DefaultPageTitle = "Demo app"
	server.IgnoreHeader("etag")
	server.PassThrough = true

	server.Get("/hello/:name", "my_layout", []string{
		"header",
		"hello",
		"footer",
	})

	server.ListenAndServe()
}

func buildLogger() *log.Logger {
	file, err := os.OpenFile("log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	defer file.Close()

	if err != nil {
		log.Fatal(err)
	}
	return log.New(io.MultiWriter(os.Stdout, file), "", log.Ldate|log.Ltime)
}

func getPort() int {
	if _, ok := os.LookupEnv("PORT"); ok {
		port, err := strconv.Atoi(os.Getenv("PORT"))

		if err != nil {
			panic(err)
		}

		return port
	}

	return 3005
}

func getTarget() string {
	if value, ok := os.LookupEnv("TARGET"); ok {
		return value
	}

	return "http://localhost:3000/_view_fragments"
}
