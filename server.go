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
	"encoding/json"
	"math/rand"
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

//Factories are defined here
type factory struct {
	Id int
	Name string
	Low int
	High int
	Members []int
	Delete bool
	Count int
}

func getAllFactories() []factory {
	result := make([]factory, 2)
	result[0] = factory {
		Id: 69,
		Name: "Jeremy",
		Low: 70,
		High: 420,
		Members: []int {286,154},
		Delete: false,
		Count: 2,
	}
	result[1] = factory {
		Id: 367,
		Name: "djw",
		Low: 42,
		High: 256,
		Members: []int {86,250,192},
		Delete: false,
		Count: 3,
	}
	return result
}

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
			fmt.Println("writing ping message")
			if err := c.WriteMessage(websocket.PingMessage, []byte{}); err != nil {
				fmt.Println("Write pong fail")
				return
			}
			c.SetWriteDeadline(time.Now().Add(writeWait))
		}
	}
}

func getIdx(f []factory, id int) int {
	//now locate the record
	//Yes, I know this is inefficient
	for idx, value := range f {
		if value.Id == id {
			return idx
		}
	}
	return -1;
}

func randInts(low int, high int, count int) []int {
	spread := high - low + 1
	result := make([]int, count)
	for i := 0; i < count; i++ {
		result[i] = low + rand.Int() % spread
	}
	return result
}

func main() {
	var listenAddr = flag.String("http", ":8080", "address to listen on for HTTP")
	flag.Parse()

	factories := getAllFactories()
	nextId := 444

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
	//Setup infinite loop to read messages
    for {
	    _, buffer, err := conn.ReadMessage()

	    if ( err != nil ){
	    	fmt.Println(err)
	    	break
	    }
	    //the incoming payload is a command
	    //parse it and send the appropriate response
	    command := string(buffer[:4])
	    switch (command) {
	    case "test":
	    	send <- []byte("Hello World")
	    case "list":
	    	if data, err := json.Marshal(factories); err == nil {
		    	send <- data	    		
	    	}
	    case "post":
	    	//unmarsal json and upsert
	    	newFactory := &factory{}
	    	id := 0
	    	update := true
	    	if err := json.Unmarshal(buffer[5:], newFactory); err == nil {
	    		//if id is zero, add the factory to the array
	    		if ( newFactory.Id == 0 ){
	    			fmt.Println("New id issued", nextId)
	    			newFactory.Id = nextId
	    			nextId++
	    			factories = append(factories, *newFactory)
	    			update = false
	    		}
		    	id = newFactory.Id
	    	} else {
	    		fmt.Println("error on unmarshal", err)
	    	}
	    	fmt.Println("id",id)
	    	//Yes, we are unmarshalling twice, once to get the id, again to update the record
    		if idx := getIdx(factories, id); idx >= 0 {
    			if update {
					json.Unmarshal(buffer[5:], &factories[idx])
					fmt.Println("update", string(buffer[5:]))
				}
				//Generate members if necessary
				if newFactory.Count > 0 {
					factories[idx].Members = randInts(factories[idx].Low, factories[idx].High, factories[idx].Count)
				}
				if data, err := json.Marshal(factories[idx:idx+1]); err == nil {
					send <- data
    			}
    		}
	   	case "retr":
	    	//parse id and return single
	    	if id, err := strconv.ParseInt(string(buffer[5:]), 10, 64); err == nil {
	    		//now locate the record
	    		if idx := getIdx(factories, int(id)); idx >= 0 {
    				if data, err := json.Marshal(factories[idx:idx+1]); err == nil {
    					send <- data
	    			}
	    		}
	    	}
	    case "dele":
	    	//parse id
	    	if id, err := strconv.ParseInt(string(buffer[5:]), 10, 64); err == nil {
	    		//now locate the record
	    		if idx := getIdx(factories, int(id)); idx >= 0 {
    				factories = append(factories[:idx], factories[idx+1:]...)
    				result := make([]factory, 1)
    				result[0] = factory{Id: int(id), Delete: true}
    				if data, err := json.Marshal(result); err == nil {
    					send <- data
    				}	    			
	    		}
	    	}
	    case "echo":
	    	send <- buffer[5:]
	    }
	}
}));


	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}