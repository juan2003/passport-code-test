package main

import (
	"net/http"
	"os"
	"flag"
	"strconv"
	"log"
	"github.com/gorilla/websocket"
	"fmt"
	"time"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer.
	maxMessageSize = 512
)

func writeLoop(c *websocket.Conn, send chan []byte) {
	fmt.Println("Entering write loop")
	//Setup ping -
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.Close()
	}()
	for {
		select {
		case message, ok := <-send:
			if !ok {
				c.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			c.SetWriteDeadline(time.Now().Add(writeWait))
			w, err := c.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued chat messages to the current websocket message.
			n := len(send)
			for i := 0; i < n; i++ {
				w.Write(<-send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				fmt.Println("Write pong fail")
				return
			}
			c.SetWriteDeadline(time.Now().Add(writeWait))
		}
	}
}

func main() {
	var listenAddr = flag.String("http", ":8080", "address to listen on for HTTP")
	flag.Parse()

	//Serve index file
	http.Handle("/test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		file, _ := os.Open("index.html")
		buffer := make([]byte,100000)
		contentLength, _ := file.Read(buffer)
		file.Close()
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("Date", "Sun, 08 May 2016 14:04:53 GMT")
		w.Header().Set("Content-Length", strconv.Itoa(contentLength))
		w.Write(buffer[:contentLength])
	}))

var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
}

http.Handle("/ws", http.HandlerFunc(func (w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Println(err)
        return
    }
    send := make(chan []byte, 256)
    //new thread to write response
    go writeLoop(conn, send)
    //close connection upon exit
	defer conn.Close()
	//setup pong handlers
	conn.SetReadLimit(maxMessageSize)
	conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error { conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })

	//Send initial payload
	send <- []byte("Hello World")
	//Setup infinite loop to read messages
    for {
	    msgType, buffer, err := conn.ReadMessage()

	    if ( err != nil ){
	    	fmt.Println(err)
	    	break
	    }
    	fmt.Println(buffer)
    	fmt.Println(msgType)
	}
}));


	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}