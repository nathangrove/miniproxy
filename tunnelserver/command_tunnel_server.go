package main

import (
	"bitbucket.org/kardianos/service"
	"bufio"
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"io"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type proxy struct {
	conn      net.Conn
	port      int
	last_beat int
	status    string
}

var command_pool = make(map[string]proxy)
var data_pool = make(map[string]net.Conn)
var db *sql.DB

/* HERE IS THE START OF THE SERVICE MANAGMENT CODE */
var logSrv service.Logger
var name = "proxy"
var displayName = "MiniProxy Tunnel Server"
var desc = "A small tunnel server for relaying HTTP reqeuests VIA socks5 proxies"
var version = "0.1"

// Runs the program as a hand or serving as a command line tool.
// Several verbs allows you to install, start, stop or remove the service.
// "Run" verb allows you to run the program as a command line tool.
// Eg "goServerView install" installs the Service
// Eg "goServerView run" starts the program from the console (blocking)
func main() {

	// setup the logging
	f, _ := os.OpenFile("/var/log/proxy.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	defer f.Close()
	log.SetOutput(f)

	s, err := service.NewService(name, displayName, desc)
	if err != nil {
		fmt.Printf("%s unable to start: %s", displayName, err)
		return
	}
	logSrv = s

	if len(os.Args) > 1 {
		var err error
		verb := os.Args[1]
		switch verb {
		case "install":
			err = s.Install()
			if err != nil {
				fmt.Printf("Failed to install: %s\n", err)
				return
			}
			fmt.Printf("Service \"%s\" installed.\n", displayName)
		case "remove":
			err = s.Remove()
			if err != nil {
				fmt.Printf("Failed to remove: %s\n", err)
				return
			}
			fmt.Printf("Service \"%s\" removed.\n", displayName)
		case "run":
			do_work()
		case "start":
			err = s.Start()
			if err != nil {
				fmt.Printf("Failed to start: %s\n", err)
				return
			}
			log.Println("Service starting")
			fmt.Printf("Service \"%s\" started.\n", displayName)
		case "stop":
			err = s.Stop()
			if err != nil {
				fmt.Printf("Failed to stop: %s\n", err)
				return
			}
			log.Println("Shutting down")
			fmt.Printf("Service \"%s\" stopped.\n", displayName)
		case "version":
			fmt.Printf(version)
			return
		}
		return
	}
	err = s.Run(func() error {
		// start
		go do_work()
		return nil
	}, func() error {
		// stop
		return nil
	})
	if err != nil {
		s.Error(err.Error())
	}
}

// lets kick things off
func do_work() {
	// connect to our database
	db, _ = sql.Open("mysql", "root:@dm1n123@/backproxy")

	defer db.Close()

	// start the http server
	// go http_server()

	// start the https server
	// go https_server()

	// start listening for proxies to call home
	proxy_listener()

}

// a function to fireup the proxy server listener...this listens for data and command tunnels being initiated
func proxy_listener() {

	log.Println("Listening for proxies")

	// LISTEN FOR CONNECTIONS
	proxy_listener, err := net.Listen("tcp", ":8081")
	if err != nil {
		log.Fatal("Proxy ListenError: ", err)
	}

	for {

		// ACCEPT THE REQUEST TO CONNECT
		proxy_conn, proxy_err := proxy_listener.Accept()
		if proxy_err != nil {
			log.Println("Proxy acception error: ", proxy_err)
		}

		// NOW HANDLE THE REQUEST
		go handle_proxy_connection(proxy_conn)
	}

}

// handler for whenever a connection is dialed in from a proxy...this is the meat and potatoes
func handle_proxy_connection(conn net.Conn) {

	// READ THE REQUEST
	request, err := bufio.NewReader(conn).ReadString('\n')
	request = strings.TrimSpace(request)

	if err != nil {
		log.Println("Reading error: ", err)
	}

	// AN ESTABLISHMENT OF A COMMAND TUNNEL
	if request == "COMMAND" {

		log.Println("Command tunnel request from " + conn.RemoteAddr().String())

		// INITIATE A CLIENT LISTENER ON AN OPEN PORT
		client_listener, err := net.Listen("tcp", ":0")
		if err != nil {
			log.Println("Setting up client listener failed")
			conn.Close()
			return
		}

		// GIVE THIS PROXY AN ID
		index := strconv.Itoa(int(time.Now().Unix())) + strconv.Itoa(rand.Intn(10000))

		// ADD THE PROXY TO THE POOL
		prox := new(proxy)
		prox.conn = conn
		prox.port = client_listener.Addr().(*net.TCPAddr).Port
		prox.last_beat = int(time.Now().Unix())
		prox.status = "ACTIVE"
		command_pool[index] = *prox

		// UPDATE THE DATABASE
		ip_parts := strings.Split(conn.RemoteAddr().String(), ":")
		res, _ := db.Exec("INSERT INTO proxy SET `proxy`.`ip`=?, proxy.type='SOCKS5', proxy.port=?, `proxy`.`status`='INITIALIZE', `proxy`.`date`=NOW()", ip_parts[0], strconv.Itoa(prox.port))
		if res == nil {
			_, _ = db.Exec("UPDATE proxy SET `proxy`.`status`='ACTIVE', port=? WHERE proxy.ip = ?", strconv.Itoa(prox.port), ip_parts[0])
		}

		// LISTEN FOR HEARTBEATS ON THE TUNNEL
		go func(prox *proxy, index string) {
			for {

				// READ REQUEST
				request, err := bufio.NewReader(prox.conn).ReadString('\n')
				request = strings.TrimSpace(request)

				// IF THERE WAS AN ERROR (EXAMPLE: WE CLOSED THE CONNECTION DUE TO TIMEOUT) RETURN
				if err != nil && err != io.EOF {
					log.Println("Command tunnel listening error: ", err)
					return
				}

				// IS IT A HEARTBEAT?
				if request == "HEARTBEAT" {
					if prox.status == "DEAD" {
						log.Println("proxy is dead")
						return
					}

					ip_parts := strings.Split(prox.conn.RemoteAddr().String(), ":")
					db.Exec("UPDATE proxy SET beat=NOW() WHERE proxy.ip = ?", ip_parts[0])
					//log.Println("Recieved heartbeat from " + prox.conn.RemoteAddr().String() + " ....responding")

					// UPDATE THE TUNNEL'S LAST HEARTBEAT
					prox.last_beat = int(time.Now().Unix())

					// SEND THE BEAT BACK
					fmt.Fprintf(prox.conn, "HEARTBEAT\n")
				}

			}
		}(prox, index)

		// ACCEPT CLIENT LISTENER CONNECTION REQUESTS
		go func(listener net.Listener, proxy_index string) {

			for {

				// FIND THE PROXY POOL TO SEE WHERE WE NEED TO DIRECT THE REQUESTS TO
				if _, ok := command_pool[proxy_index]; !ok {

					// DOESNT LOOK LIKE WE HAVE A PROXY WITH THAT ID
					log.Println("Still listening on a dead proxy...closing the listener")
					listener.Close()
					return

				}

				// ACCEPT THE REQUEST TO CONNECT
				client_conn, _ := listener.Accept()

				// NOW HANDLE THE REQUESTS
				go handle_client_connection(client_conn, proxy_index)
			}

		}(client_listener, index)

		// START A WATCHDOG FOR THE TUNNEL
		go command_watchdog(prox, index)

		// DONE
		log.Println("Command connection " + index + " from " + conn.RemoteAddr().String() + " listening on " + strconv.Itoa(prox.port))

		// MUST BE A DATA TUNNEL
	} else {
		log.Println("Data tunnel #" + request + " request")

		// VERIFY THE DATA TUNNEL...
		if client, ok := data_pool[request]; ok {

			// BIND THE CLIENT CONNECTION TO THE DATA TUNNEL
			go io.Copy(client, conn)
			go io.Copy(conn, client)

			// MONITOR THE STREAM FOR ACTIVITY
			go watch_stream(client, conn, request)

			// COULD NOT VERIFY IT AS A DATA TUNNEL
		} else {
			log.Println("Data tunnel not found")
		}
	}

}

// handler for whenever a client dials in
func handle_client_connection(client net.Conn, proxy_index string) {

	var proxy proxy

	// FIND THE PROXY POOL TO SEE WHERE WE NEED TO DIRECT THE REQUESTS TO
	if val, ok := command_pool[proxy_index]; ok {

		// FOUND IT
		proxy = val

	} else {

		// DOESNT LOOK LIKE WE HAVE A PROXY WITH THAT ID
		log.Println("Command proxy does not exist")
		return
	}

	// NOW WE HAVE THE PROXY AND CONNECTION...LETS TELL THE PROXY TO CREATE A DATA TUNNEL FOR US

	// NAME THE DATA TUNNEL
	index := strconv.Itoa(int(time.Now().Unix())) + "-" + strconv.Itoa(rand.Intn(10000))
	data_pool[index] = client
	log.Println("Connection client: " + client.RemoteAddr().String() + " to " + proxy_index + " assigned data tunnel #" + index)

	// ISSUE THE COMMAND
	fmt.Fprintf(proxy.conn, index+"\n")

}

// keep checking for the last heartbeat...if it has been too long...declare it dead
func command_watchdog(proxy *proxy, index string) {

	// keep checking for for timeout every second....
	ticker := time.NewTicker(time.Second * 1)

	for _ = range ticker.C {

		// IS THE LAST BEAT MORE THAN 20 SECONDS OLD?
		if proxy.last_beat < int(time.Now().Unix())-20 {
			log.Println("Tunnel watchdog detected a timeout on command tunnel #" + index)

			// UPDATE THE DATABASE
			ip_parts := strings.Split(proxy.conn.RemoteAddr().String(), ":")
			_, _ = db.Exec("UPDATE proxy SET `proxy`.`status`='INACTIVE' WHERE proxy.ip = ?", ip_parts[0])

			// CLOSE THE CONNECTION
			proxy.conn.Close()
			proxy.status = "DEAD"

			// DELETE IT FROM THE POOL
			delete(command_pool, index)

			return
		}

	}
}

// a function to watch a stream and time out the data tunnel if no data comes through
func watch_stream(client net.Conn, proxy net.Conn, data_pool_index string) {

	last_data := int(time.Now().Unix())

	go func(client net.Conn, proxy net.Conn, data_pool_index string, last_data *int) {
		for {
			// try to read the data
			data := make([]byte, 512)
			//log.Println("Read data: ", data)

			_, err := proxy.Read(data)
			if err != nil {
				if err.Error() != "use of closed network connection" {
					log.Println("Connection read error : '" + err.Error() + "' Closing data tunnel #" + data_pool_index)
					delete(data_pool, data_pool_index)
					client.Close()
					client = nil
					return
				}
			}
			//log.Println("Updaing last read time on data tunnel #" + data_pool_index)
			*last_data = int(time.Now().Unix())
		}
	}(client, proxy, data_pool_index, &last_data)

	// keep checking for for timeout every 30 seconds....
	ticker := time.NewTicker(time.Second * 2)

	for t := range ticker.C {

		// log.Println("Checking for timeout")
		// 30 second timeout
		if int(t.Unix()) > last_data+30 {
			client.Close()
			client = nil
			delete(data_pool, data_pool_index)
			log.Println("Timeout on data tunnel #" + data_pool_index)
			return
		}

	}

}

// listen for client requests
func client_listener() {

	log.Println("Listening for clients")
	// ACCEPT CLIENT CONNECTION
	client_listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		log.Fatal("Client ListenError: ", err)
	}

	for {
		client_conn, client_err := client_listener.Accept()
		if client_err != nil {
			log.Println("Client acception error: ", client_err)
		}
		go handle_client_connection(client_conn, "1234")
	}

}
