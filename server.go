package main

import (
	"net/http"
	"os"
	"flag"
	"strconv"
	"log"
	"github.com/gorilla/websocket"
	"time"
	"encoding/json"
	"math/rand"
	"sync"
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

// hub maintains the set of active connections and broadcasts messages to the
// connections.
type Hub struct {
	// Registered connections.
	connections map[*Conn]bool

	// Inbound messages from the connections.
	broadcast chan []byte

	// Register requests from the connections.
	register chan *Conn

	// Unregister requests from connections.
	unregister chan *Conn
}

var hub = Hub{
	broadcast:   make(chan []byte),
	register:    make(chan *Conn),
	unregister:  make(chan *Conn),
	connections: make(map[*Conn]bool),
}

func (h *Hub) run() {
	for {
		select {
		case conn := <-h.register:
			h.connections[conn] = true
		case conn := <-h.unregister:
			if _, ok := h.connections[conn]; ok {
				delete(h.connections, conn)
				close(conn.send)
			}
		case message := <-h.broadcast:
			for conn := range h.connections {
				select {
				case conn.send <- message:
				default:
					close(conn.send)
					delete(hub.connections, conn)
				}
			}
		}
	}
}

//Factories are defined here
type Factory struct {
	Id int
	Name string
	Low int
	High int
	Members []int
	Delete bool
	Count int
}

func getAllFactories() []Factory {
	result := make([]Factory, 2)
	result[0] = Factory {
		Id: 69,
		Name: "Jeremy",
		Low: 70,
		High: 420,
		Members: []int {286,154},
		Delete: false,
		Count: 2,
	}
	result[1] = Factory {
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

func getIdx(f []Factory, id int) int {
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

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
}

// Conn is an middleman between the websocket connection and the hub.
type Conn struct {
	// The websocket connection.
	ws *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte
}

// readPump pumps messages from the websocket connection to the hub.
func (c *Conn) readPump() {
    //close connection upon exit
	defer func() {
		hub.unregister <- c
		c.ws.Close()
	}()
	c.ws.SetReadLimit(maxMessageSize)
	c.ws.SetReadDeadline(time.Now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error { c.ws.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	//Setup infinite loop to read messages
    for {
	    _, buffer, err := c.ws.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway) {
				log.Printf("error: %v", err)
			}
			break
		}

	    //the incoming payload is a command
	    //parse it and send the appropriate response
	    command := string(buffer[:4])
	    switch (command) {
	    case "test":
	    	//Only goes back to sender
	    	c.send <- []byte("Hello World")
	    case "list":
	    	if data, err := json.Marshal(factories); err == nil {
		    	c.send <- data	    		
	    	}
	    case "post":
	    	//unmarsal json and upsert
	    	newFactory := &Factory{}
	    	id := 0
	    	update := true
	    	if err := json.Unmarshal(buffer[5:], newFactory); err == nil {
	    		//if id is zero, add the factory to the array
	    		if ( newFactory.Id == 0 ){
	    			newFactory.Id = nextId.next()
	    			factoryLock.Lock()
	    			factories = append(factories, *newFactory)
	    			factoryLock.Unlock()
	    			update = false
	    		}
		    	id = newFactory.Id
	    	}
	    	//Yes, we are unmarshalling twice, once to get the id, again to update the record
			factoryLock.Lock()
    		if idx := getIdx(factories, id); idx >= 0 {
    			if update {
					json.Unmarshal(buffer[5:], &factories[idx])
				}
				//Generate members if necessary
				if newFactory.Count > 0 {
					factories[idx].Members = randInts(factories[idx].Low, factories[idx].High, factories[idx].Count)
				}
				if data, err := json.Marshal(factories[idx:idx+1]); err == nil {
					//New update goes to everyone
					hub.broadcast <- data
    			}
    		}
			factoryLock.Unlock()
	   	case "retr":
	    	//parse id and return single
	    	if id, err := strconv.ParseInt(string(buffer[5:]), 10, 64); err == nil {
	    		//now locate the record
	    		if idx := getIdx(factories, int(id)); idx >= 0 {
    				if data, err := json.Marshal(factories[idx:idx+1]); err == nil {
    					c.send <- data
	    			}
	    		}
	    	}
	    case "dele":
	    	//parse id
	    	if id, err := strconv.ParseInt(string(buffer[5:]), 10, 64); err == nil {
    			factoryLock.Lock()
	    		if idx := getIdx(factories, int(id)); idx >= 0 {
    				factories = append(factories[:idx], factories[idx+1:]...)
    				result := make([]Factory, 1)
    				result[0] = Factory{Id: int(id), Delete: true}
    				if data, err := json.Marshal(result); err == nil {
    					hub.broadcast <- data
    				}	    			
	    		}
    			factoryLock.Unlock()
	    	}
	    case "echo":
	    	c.send <- buffer[5:]
	    }
	}
}

// write writes a message with the given message type and payload.
func (c *Conn) write(mt int, payload []byte) error {
	c.ws.SetWriteDeadline(time.Now().Add(writeWait))
	return c.ws.WriteMessage(mt, payload)
}

// writePump pumps messages from the hub to the websocket connection.
func (c *Conn) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.ws.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				c.write(websocket.CloseMessage, []byte{})
				return
			}

			c.ws.SetWriteDeadline(time.Now().Add(writeWait))
			w, err := c.ws.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Add queued chat messages to the current websocket message.
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write(newline)
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.write(websocket.PingMessage, []byte{}); err != nil {
				return
			}
		}
	}
}

var factories = getAllFactories()
var factoryLock sync.Mutex

type IdCounter struct {
	id int
	mux sync.Mutex
}

func (idc *IdCounter) next() int {
	idc.mux.Lock()
	defer idc.mux.Unlock()
	val := idc.id
	idc.id++
	return val
}

var nextId = IdCounter { id: 401 }


func main() {
	var listenAddr = flag.String("http", ":8080", "address to listen on for HTTP")
	flag.Parse()
	go hub.run()
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



http.Handle("/ws", http.HandlerFunc(func (w http.ResponseWriter, r *http.Request) {
    ws, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Println(err)
        return
    }
	conn := &Conn{send: make(chan []byte, 256), ws: ws}
	hub.register <- conn
	go conn.writePump()
	conn.readPump()
}));


	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}