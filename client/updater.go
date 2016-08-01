package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"
)

func main() {

	// check installed version
	path := "C:\\Program Files\\MiniProxy\\proxy.exe"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		path = "C:\\Program Files (x86)\\MiniProxy\\proxy.exe"
	}

	// download the new version
	// update current exe file
	out, err := os.Create(path + ".partial")
	if err != nil {
		log.Println("File creation failed: ", err)
		return
	}

	defer out.Close()
	resp, err := http.Get("http://proxy.userwatchdog.com/proxy.exe")
	if err != nil {
		log.Println("File GET failed: ", err)
		return
	}

	defer resp.Body.Close()
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return
	}

	out.Close()

	os.Remove(path)

	os.Rename(path+".partial", path)
	time.Sleep(time.Second)

	c := exec.Command("net", "start", "MiniProxy")
	if err := c.Run(); err != nil {
		log.Println("Starting MiniProxy Error: ", err)
	}

	//}

}
