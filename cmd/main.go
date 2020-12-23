package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	gemini "github.com/LukeEmmet/net-gemini"
)

//_________________________________________________
//very simplistic home page, advising the visitor to go somewhere more specific
var homePageGMI = "" + 
`# Geminigem home page

A dynamic server for scripting gemini and nimigem protocols.

But there's not much to see on this home page, so please go somewhere more specific

Thank you.`
//_________________________________________________



func homePage (w *gemini.Response, r *gemini.Request) {

		w.SetStatus(gemini.StatusSuccess, "text/gemini")
		w.Write(([]byte(homePageGMI)))
	}
    
func main() {
	cgi := flag.String("cgi", "cgi-bin", "cgi directory")
	crt := flag.String("crt", "", "path to cert")
	key := flag.String("key", "", "path to cert key")
	bind := flag.String("bind", "localhost:1965", "bind to")
	flag.Parse()

	fmt.Fprintln(os.Stderr, "starting up")

	//use CGI module
    gemini.Handle("/cgi-bin", gemini.CGIServer(*cgi, *bind))

    //simple home/default page otherwise
	gemini.HandleFunc("/", homePage)
    
	log.Fatal(gemini.ListenAndServeTLS(*bind, *crt, *key))
}
