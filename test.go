package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
)

var host = flag.String("host", "localhost", "Host Ip adress")
var port = flag.String("port", "3333", "Port")

var (
	apiConnection *websocket.Conn
)

func connectApi(terminal chan<- string) *websocket.Conn {

	u := url.URL{Scheme: "ws", Host: *host + ":" + *port, Path: "/ws"}

	for {
		ac, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
		if err != nil {
			fmt.Println("connexion error")
			time.Sleep(3 * time.Second)
			continue
		}
		apiConnection = ac
		break
	}
	fmt.Println("connected")
	time.Sleep(8 * time.Second)
	return apiConnection
}
func main() {

	flag.Parse()
	log.SetFlags(0)

	terminal_output := make(chan string, 1)
	defer close(terminal_output)
	terminal_output <- ""

	c := connectApi(terminal_output)
	defer c.Close()

	// test connexion & reconnexoin

	go func() {
		for {
			_, _, err := c.ReadMessage()
			if err != nil {
				fmt.Println("connexion missed")
				time.Sleep(8 * time.Second)
				c = connectApi(terminal_output)
				defer c.Close()
				return
			}
		}
	}()

}
