package main

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"golang.org/x/net/websocket"
)

func main() {
	fmt.Printf("Your endpoint: ")

	var endpoint string
	fmt.Scanln(&endpoint)

	socket, err := websocket.Dial("ws://localhost:3001/ws", "", fmt.Sprintf("http://localhost:%s", endpoint))
	if err != nil {
		panic(err)
	}

	serverClosed := make(chan struct{})

	go subscribeSocket(socket, serverClosed)
	go sendToServer(socket)

	for range serverClosed {
		fmt.Println("Server is closed")

		close(serverClosed)
	}
}

// SubscribeSocket waits for the response from the client.
// If the server is closed, this will push some data to the serverChan to signal the program to terminate.
func subscribeSocket(ws *websocket.Conn, serverChan chan struct{}) {
	for {
		var message string

		err := websocket.Message.Receive(ws, &message)
		if err == io.EOF {
			serverChan <- struct{}{}
			return
		} else if err != nil {
			panic(err)
		}

		fmt.Println(message)
	}
}

// sendToServer sends message from the client to the server.
func sendToServer(ws *websocket.Conn) {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		scanner.Scan()

		msg := scanner.Text()
		ws.Write([]byte(msg))
	}
}
