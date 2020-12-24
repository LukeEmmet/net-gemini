package gemini

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"
)

type Handler interface {
	ServeGemini(*Response, *Request)
}

type Server struct {
	Addr         string
	Handler      Handler
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	listener     net.Listener
	Log          io.Writer
}

func (s *Server) logf(format string, args ...interface{}) {
	if s.Log != nil {
		now := fmt.Sprintf("%v ", time.Now().Format(time.ANSIC))
		fmt.Fprintf(s.Log, now+format+"\n", args...)
	}
}

func (s *Server) ListenAndServeTLS(certFile, keyFile string) error {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return err
	}
	tlscfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	listener, err := tls.Listen("tcp", s.Addr, tlscfg)
	if err != nil {
		return err
	}
	defer listener.Close()

	s.logf("server listening on %s [tls: %v %v]", listener.Addr(), certFile, keyFile)
	return s.serve(listener)
}

func (s *Server) serve(listener net.Listener) error {
	s.listener = listener

	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}

		go s.handleGeminiRequest(conn)
	}
}

func (s *Server) handleGeminiRequest(conn net.Conn) {
	readDeadline := time.Time{}
	in := Request{}
	out := Response{conn: conn}
	t0 := time.Now()
	if d := s.ReadTimeout; d != 0 {
		readDeadline = t0.Add(d)
		conn.SetReadDeadline(readDeadline)
	}
	// FIXME: this is read + write, should be only write
	if d := s.WriteTimeout; d != 0 {
		conn.SetWriteDeadline(time.Now().Add(d))
	}

	// FIXME: use something else
	defer func() {
		s.logf("%s -> %s request: %s took %v, status: %v %v", conn.RemoteAddr(), conn.LocalAddr(), in.URL, time.Since(t0), out.statusCode, out.statusText)
	}()

	defer conn.Close()


	//we'll read the first 10 bytes to determine what is the scheme of the URL,
	//to at least distinguish gemini from nimigem
	snippetSize := 10
	requestHead := make([]byte, snippetSize)
	reader := bufio.NewReaderSize(conn, snippetSize)
	io.ReadFull(reader, requestHead)


	scheme := strings.Split(string(requestHead), ":")[0]	//extract the scheme (gemini requires full scheme to be provided)
	maxRequest := 1024		//default URL max size for gemini
	switch string(scheme) {
		case "gemini":
			break
		case "nimigem":
			maxRequest = 15360		//15kb encoded request equates to approx 10kb content (average gemini posting is 5k)
			break
		default:
			out.SetStatus(StatusPermanentFailure, "Unknown or missing URL scheme. Only gemini and nimigem are supported!")
			return
	}

	reader = bufio.NewReaderSize(reader, maxRequest - snippetSize)		//use larger buffer now up to size permitted by the scheme
	requestTail, overflow, err := reader.ReadLine()		//get the rest of the line

	if overflow {
		_ = out.SetStatus(StatusPermanentFailure, "Request too long!")
		return
	} else if err != nil {
		_ = out.SetStatus(StatusTemporaryFailure, "Unknown error reading request! "+err.Error())
		return
	}

	payload := ""		//defined for nimigem, otherwise empty for gemini
	urlPart := ""
	request := string(requestHead) + string(requestTail)

	//check if it is a nimigem post payload, and split on the space if so
	if scheme == "nimigem" {
		parts := strings.Split(request, " ")
		urlPart = parts[0]

		if len(parts) > 1 {
			payload, err = url.PathUnescape(parts[1])		//decode the payload
			if err != nil {
				_ = out.SetStatus(StatusPermanentFailure, "invalid nimigem payload encoding! "+err.Error())
			}
		}
	} else {
		urlPart = request
	}

	//parse the URL
    URL, err := url.Parse(urlPart)
	if err != nil {
		_ = out.SetStatus(StatusPermanentFailure, "Error parsing URL! "+err.Error())
		return
	}


	in.URL = URL
	in.Payload = payload

    //hand off to the active handler for this request
	s.Handler.ServeGemini(&out, &in)
}

func (s *Server) Close() {
	s.listener.Close()
}
