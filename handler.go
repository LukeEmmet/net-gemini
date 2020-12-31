package gemini

import (
	"fmt"
	"net"
	"net/url"
)

type Request struct {
	URL *url.URL
	Payload string
}

type Response struct {
	Conn       net.Conn
	status     bool
	statusCode Status
	statusText string
}

func (r *Response) Write(b []byte) (int, error) {
	if !r.status {
		err := r.SetStatus(StatusSuccess, "text/gemini")
		if err != nil {
			return 0, err
		}
	}
	return r.Conn.Write(b)
}

func (r *Response) SetStatus(s Status, mime string) error {
	if r.status {
		r.statusCode = s
		r.statusText = mime
		return nil
	} else {
		_, err := r.Conn.Write([]byte(fmt.Sprintf("%d %s\r\n", s, mime)))
		if err == nil {
			r.status = true
			r.statusCode = s
			r.statusText = mime
		}
		return err
	}

}
