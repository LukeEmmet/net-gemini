package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	gemini "github.com/LukeEmmet/net-gemini"
)

var version = "0.1.1"
var serverName = "Demo Dynamic Gemini/Nimigem server"

func infoPage(w *gemini.Response, r *gemini.Request) {

	info := "# Server info\n" +
		"\n" +
		"* server name: " + serverName + "\n" +
		"* version: " + version + "\n" +
		"* local time: " + time.Now().Format(time.ANSIC) + "\n" +
		""

	w.SetStatus(gemini.StatusSuccess, "text/gemini")
	w.Write(([]byte(info)))
}

func exampleHandler(w *gemini.Response, r *gemini.Request) {
	if len(r.URL.RawQuery) == 0 {
		w.SetStatus(gemini.StatusInput, "what is the answer to the ultimate question")
	} else {
		w.SetStatus(gemini.StatusSuccess, "text/gemini")
		answer := r.URL.RawQuery
		w.Write([]byte("HELLO: " + r.URL.Path + ", yes the answer is: " + answer))
	}
}

func main() {
	root := flag.String("root", "docs", "root directory")
	cgi := flag.String("cgi", "cgi-bin", "cgi directory")
	crt := flag.String("crt", "", "path to cert")
	key := flag.String("key", "", "path to cert key")
	bind := flag.String("bind", "localhost:1965", "bind to")
	flag.Parse()

	fmt.Fprintln(os.Stderr, "Starting up Geminigem demo server on "+*bind)

	//examples of a custom function handler
	gemini.HandleFunc("/example", exampleHandler)
	gemini.HandleFunc("/info", infoPage)

	//use cgi module to handle urls starting cgi-bin
	gemini.Handle("/cgi-bin", gemini.CGIServer(*cgi, serverName))

	//put the generic one last, otherwise it will take precedence over others
	//file handling module is
	gemini.Handle("/", gemini.FileServer(*root))

	log.Fatal(gemini.ListenAndServeTLS(*bind, *crt, *key))
}
