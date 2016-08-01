package main

import (
	"bitbucket.org/kardianos/service"
	"bufio"
	"fmt"
	//"github.com/elazarl/goproxy"
	"github.com/armon/go-socks5"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

/* HERE IS THE START OF THE SERVICE MANAGMENT CODE */
var logSrv service.Logger
var name = "MiniProxy"
var displayName = "Mini Proxy"
var desc = "A small backproxy for relaying HTTP reqeuests"
var version = "0.5"

// Runs the program as a hand or serving as a command line tool.
// Several verbs allows you to install, start, stop or remove the service.
// "Run" verb allows you to run the program as a command line tool.
// Eg "goServerView install" installs the Service
// Eg "goServerView run" starts the program from the console (blocking)
func main() {
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
			fmt.Printf("Service \"%s\" started.\n", displayName)
		case "stop":
			err = s.Stop()
			if err != nil {
				fmt.Printf("Failed to stop: %s\n", err)
				return
			}
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
		stop_work()
		return nil
	})
	if err != nil {
		s.Error(err.Error())
	}
}

var continue_running = 1

func do_work() {

	// start a function to check for updates
	go update_monitor()

	// every second lets dial the tunnel server...forever
	for _ = range time.Tick(time.Second) {

		// Create a SOCKS5 server
		conf := &socks5.Config{}
		server, err := socks5.New(conf)
		if err != nil {
			panic(err)
		}

		// Create SOCKS5 proxy on localhost port 8000
		go server.ListenAndServe("tcp", "127.0.0.1:12345")

		// dial tunnel server
		tunnel, err := net.Dial("tcp", "proxy.userwatchdog.com:8081")
		if err != nil {
			log.Println("Connection dialing failed: ", err)
			continue
		}

		// initate the tunnel as a command tunnel
		_, err = fmt.Fprintf(tunnel, "COMMAND\n")
		if err != nil {
			log.Println("Could not write to the command tunnel")
			continue
		}

		log.Println("Command tunnel established on socket: " + tunnel.LocalAddr().String())

		// we just connected...so we will say we just got a heartbeat from the tunnelserver
		last_beat := int(time.Now().Unix())

		// start the watchdog....watch for heartbeats from the tunnel server
		go tunnel_watchdog(tunnel, &last_beat)

		// send heartbeats to the tunnelserver
		go tunnel_beat(tunnel)

		// read forever on the command tunnel...it will tell us when to establish a data tunnel
		for {

			// read input
			request, err := bufio.NewReader(tunnel).ReadString('\n')
			if err != nil {
				log.Println("Reading error: ", err)
				break
			}
			request = strings.TrimSpace(request)

			// is it a heartbeat?
			if request == "HEARTBEAT" {
				log.Println("Heartbeat recieved")
				last_beat = int(time.Now().Unix())
				continue

				// is it a data tunnel?
			} else if strings.Index(request, "-") > -1 {

				// check if it is an integer
				parts := strings.Split(request, "-")
				if _, err := strconv.Atoi(parts[0]); err != nil {
					log.Println("Bad timestamp", err)
					return
				} else if _, err := strconv.Atoi(parts[1]); err != nil {
					log.Println("Bad tunnel ID", err)
					return
				} else {
					// valid data tunnel id...lets respond and make the tunnel
					data_tunnel(request)
				}

				// unknown request
			} else {
				log.Println("Unknown request from comamnd tunnel: " + request)
			}

		}
	}

}

func stop_work() {
	continue_running = 0
}

// a function to create a data tunnel and bind it to the proxy
func data_tunnel(request string) {

	log.Println("Creating data tunnel #" + request)

	// dial server
	tunnel, err := net.Dial("tcp", "proxy.userwatchdog.com:8081")
	if err != nil {
		log.Println("Data connection dialing failed: ", err)
	} else {
		log.Println("Data tunnel cretaed on socket: " + tunnel.LocalAddr().String())
	}

	// let the tunnel server know which data tunnel we are...
	fmt.Fprintf(tunnel, request+"\n")

	// sleep for a second so it can see hold on for half a second
	time.Sleep(time.Nanosecond * 500000000)

	// dial the proxy...
	bridge, err := net.Dial("tcp", "127.0.0.1:12345")
	if err != nil {
		log.Fatal("Bridge connection failed: ", err)
	}

	log.Println("Bridging port " + tunnel.LocalAddr().String() + " to " + bridge.LocalAddr().String())

	// bridge the proxy and tunnel
	go io.Copy(tunnel, bridge)
	go io.Copy(bridge, tunnel)

}

// watch for beats from the tunnel server
func tunnel_watchdog(conn net.Conn, last_beat *int) {
	// keep checking for for timeout every 30 seconds....
	ticker := time.NewTicker(time.Second * 1)

	for _ = range ticker.C {
		// if the last beat is more than 20 seconds old....then lets terminate the connection because it appears to be dead
		if *last_beat < int(time.Now().Unix())-20 {
			log.Println("Timeout detected on command tunnel")
			conn.Close()
			return
		}
	}

}

// send heartbeats through the command tunnel every 15 seconds
func tunnel_beat(conn net.Conn) {
	ticker := time.NewTicker(time.Second * 5)

	for _ = range ticker.C {
		log.Println("Sending heartbeat")
		_, err := fmt.Fprintf(conn, "HEARTBEAT\n")
		if err != nil {
			return
		}
	}
}

// function to check the server for an updated proxy.exe
func check_for_updates() {

	path := "C:\\Program Files\\MiniProxy\\updater.exe"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = "C:\\Program Files (x86)\\MiniProxy\\updater.exe"
	}

	// find out the current version
	resp, err := http.Get("http://proxy.userwatchdog.com/version.txt")
	if err != nil {
		log.Println("Version check failed: ", err)
		return
	}
	new_version_string, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()

	newest_version, err := strconv.ParseFloat(string(new_version_string), 32)
	if err != nil {
		log.Println("Converting version string to float failed: ", err)
		return
	}

	current_version, err := strconv.ParseFloat(version, 32)
	if err != nil {
		log.Println("Converting version string to float failed: ", err)
		return
	}

	if current_version < newest_version {
		log.Println("Updating...")
		command := exec.Command(path)
		command.Start()
		os.Exit(0)

	}

}

// monitor the server for updated proxy.exe
func update_monitor() {
	check_for_updates()
	for _ = range time.Tick(time.Hour) {
		check_for_updates()
	}
}
