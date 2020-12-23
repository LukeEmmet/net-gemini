# Readme for Geminigem server

Geminigem is a simple server to illustrate features of server framework

See source for further info

## Static files

docs/ folder contains home page and static files, served from gemini://server/docs/*

## CGI scripting

cgi-bin/ folder for CGI scripts, served and executed from gemini://server/cgi-bin/*

## Server info

Server info is served from gemini://server/info

## Usage

Command line parameters:

  -bind string
        bind to (default "localhost:1965")
  -cgi string
        cgi directory (default "cgi-bin")
  -crt string
        path to cert
  -docs string
        docs directory (default "docs")
  -key string
        path to cert key
