package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"time"

	"golang.org/x/net/websocket"
)

type Server struct {
	conns map[*websocket.Conn]bool
}

func NewServer() *Server {
	return &Server{
		conns: make(map[*websocket.Conn]bool),
	}
}

func (s *Server) HandleWS(ws *websocket.Conn) {
	fmt.Printf("Receive new connection from client: %s\n", ws.RemoteAddr())

	s.conns[ws] = true
	fmt.Println("Active connection: ", len(s.conns))

	buf := make([]byte, 1024000)

	// After the connection is established, the server will wait for the client's message via the socket.
	// The server will reply with "Thank you for the message!!!" upon receiving a message.
	for {
		n, err := ws.Read(buf)
		// This will occur when the connection is close from the other side.
		if err == io.EOF {
			fmt.Printf("Connection %s close...\n", ws.RemoteAddr().String())
			delete(s.conns, ws)
			break
		} else if err != nil {
			fmt.Println("read error: ", err)
			continue
		}

		msg := fmt.Sprintf("[%s] %s", ws.RemoteAddr().String(), buf[:n])
		fmt.Println("Incoming message: ", msg)

		// Broadcast to all the active user in the group.
		for userInGroup, active := range s.conns {
			if !active {
				continue
			}

			if userInGroup.RemoteAddr().String() == ws.RemoteAddr().String() {
				continue
			}

			err = websocket.Message.Send(userInGroup, msg)
			if err != nil {
				fmt.Println("Failed to send via socket: ", err)

				return
			}
		}
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fmt.Println("Starting websocket server...")
	server := NewServer()

	http.Handle("/ws", websocket.Handler(server.HandleWS))

	srvErr := make(chan error, 1)
	go func() {
		srvErr <- http.ListenAndServe(":3001", nil)
	}()

	select {
	case err := <-srvErr:
		// Error when the server starts.
		panic(err)
		// return
	case <-ctx.Done():
		// Wait for the first Ctrl+C.
		// Stop receiving signal notifications as soon as possible.
		fmt.Println("Gracefully shutdown...")
		time.Sleep(1 * time.Second)
		stop()
	}
}
