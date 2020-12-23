package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	gemini "github.com/LukeEmmet/net-gemini"
)

var version = "0.1.1"

func infoPage (w *gemini.Response, r *gemini.Request) {
	
    info := "#Geminem server info\n" +
        "\n" +
        "* Gemingem v" + version + "\n" 

    w.SetStatus(gemini.StatusSuccess, "text/gemini")
    w.Write(([]byte(info)))
}

func main() {
	//root := flag.String("root", "", "root directory")
	cgi := flag.String("cgi", "cgi-bin", "cgi directory")
	docs := flag.String("docs", "docs", "docs directory")
	crt := flag.String("crt", "", "path to cert")
	key := flag.String("key", "", "path to cert key")
	bind := flag.String("bind", "localhost:1965", "bind to")
	flag.Parse()

	//gemini.HandleFunc("/example", handler)
	fmt.Fprintln(os.Stderr, "Starting up Geminigem server on " + *bind)

	//use cgi module to handle urls starting cgi-bin
	gemini.Handle("/cgi-bin", gemini.CGIServer(*cgi, *bind))

    //handle /info with specific function 
    gemini.HandleFunc("/info", infoPage)
    
	//put the generic one last, otherwise it will take precedence over others
    //file handling module is 
	gemini.Handle("/", gemini.FileServer(*docs))

	log.Fatal(gemini.ListenAndServeTLS(*bind, *crt, *key))
}
