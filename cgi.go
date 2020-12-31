package gemini

import (
	"bufio"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
	"context"
)

//some elements taken and adapted from Molly Brown  CGI implementation

type cgiHandler struct {
	cgiRoot string
	serverName string
}

func CGIServer(root string, serverName string) Handler {
	return &cgiHandler{
		cgiRoot: filepath.Clean(root),
		serverName: serverName,
	}
}

func prepareCGIVariables(URL *url.URL, handler cgiHandler, conn net.Conn, script_path string, path_info string) map[string]string {
	vars := prepareGatewayVariables( URL, handler, conn)
	vars["GATEWAY_INTERFACE"] = "CGI/1.1"
	vars["SCRIPT_PATH"] = script_path
	vars["PATH_INFO"] = path_info
	return vars
}

func prepareGatewayVariables(URL *url.URL, handler cgiHandler, conn net.Conn) map[string]string {
	vars := make(map[string]string)
	vars["QUERY_STRING"] = URL.RawQuery
	vars["REMOTE_ADDR"] = conn.RemoteAddr().String()
	vars["REQUEST_METHOD"] = ""

	vars["SERVER_NAME"] = URL.Host
	vars["SERVER_PORT"] = URL.Port()
	vars["SERVER_PROTOCOL"] = URL.Scheme
	vars["SERVER_SOFTWARE"] = handler.serverName

	// Add TLS variables
	var tlsConn (*tls.Conn) = conn.(*tls.Conn)
	connState := tlsConn.ConnectionState()
	//	vars["TLS_CIPHER"] = CipherSuiteName(connState.CipherSuite)

	// Add client cert variables
	clientCerts := connState.PeerCertificates
	if len(clientCerts) > 0 {
		cert := clientCerts[0]
		vars["TLS_CLIENT_HASH"] = getCertFingerprint(cert)
		vars["TLS_CLIENT_ISSUER"] = cert.Issuer.String()
		vars["TLS_CLIENT_ISSUER_CN"] = cert.Issuer.CommonName
		vars["TLS_CLIENT_SUBJECT"] = cert.Subject.String()
		vars["TLS_CLIENT_SUBJECT_CN"] = cert.Subject.CommonName
	}
	return vars
}

func getCertFingerprint(cert *x509.Certificate) string {
	hash := sha256.Sum256(cert.Raw)
	fingerprint := hex.EncodeToString(hash[:])
	return fingerprint
}

func getShebang(scriptPath string) (path string, flags string) {

	exePath := ""
	exeFlags := ""

	file, _ := os.Open(scriptPath)
	defer file.Close()

	scanner := bufio.NewScanner(file)
	scanner.Scan()		//just get first line
	firstLine := scanner.Text()


	//very simplistic shebang parsing
	if firstLine[:2] == "#!" {
		exePathFlags := firstLine[2:]
		pathArray := strings.Split(exePathFlags, " ")
		exePath = pathArray[0]
		if len(pathArray) > 0 {
			exeFlags = pathArray[1]
		} else {
			exeFlags = ""
		}
	}

	flags = exeFlags
	path = exePath

	return path, flags

}

func Debug(s string) {
	fmt.Fprintf(os.Stderr, "%s\n", s)
}

func ServeCGI(p string, w *Response, r *Request, handler cgiHandler) {
	s, err := os.Stat(p)
	if err != nil {
		w.SetStatus(StatusNotFound, "File Not Found!")
		return
	}
	if !cgiAllowed(s) {
		w.SetStatus(StatusGone, "Forbidden!")
		return
	}

	URL := r.URL
	exePath := p
	exeFlags := ""

	// Spawn process
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, exePath)		//unix style - just execute the script
	if runtime.GOOS == "windows" {
		//assume it is a script - read the first line to get the interpreter
		exePath, exeFlags = getShebang(p)
		cmd = exec.CommandContext(ctx, exePath, exeFlags, p)
	}

	// Prepare environment variables
	vars := prepareCGIVariables(URL, handler, w.Conn, exePath, p)
	cmd.Env = []string{}

	if URL.Scheme == "nimigem" {
		//pass the payload on stdin to the cgi app, like HTTP POST
		cmd.Stdin =   strings.NewReader(r.Payload)	//pass in the payload on stdin, like HTTP CGI does with POST
	}
	for key, value := range vars {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	response, err := cmd.Output()

	if ctx.Err() == context.DeadlineExceeded {
		Debug("Terminating CGI process " + p + " due to exceeding 10 second runtime limit.")
		w.SetStatus(StatusCGIError, "CGI process timed out!")
		return
	}
	if err != nil {
		Debug("Error running CGI program " + p + ": " + err.Error())
		if err, ok := err.(*exec.ExitError); ok {
			Debug("↳ stderr output: " + string(err.Stderr))
		}
		w.SetStatus(StatusCGIError, "CGI error!")
		return
	}

	//there is no raw write on this server, so we need to extract the heade and body
	contentReader := bufio.NewReader(strings.NewReader(string(response)))
	header, err := getHeader(contentReader)

	if err != nil {
		w.SetStatus(StatusCGIError, fmt.Sprintf("CGI error - invalid or missing header: %s", err))
		return
	}

	//split the header into status and text
	statusSplit := strings.Split(string(header), " ")

	statusText := strings.Join(statusSplit[1:], " ")
	statusInt, err := strconv.Atoi(statusSplit[0])
	if err != nil {
		w.SetStatus(StatusCGIError, "CGI error - invalid status!")
		return
	}

	//setting w.status and w.statuscode - these arent writeable, need to set explicitly thus
	w.SetStatus(Status(statusInt), statusText)

	//the body is everything beyond the header text + /r/n
	w.Write(response[2 + len(header):])
}

func getHeader (contentReader *bufio.Reader) (header string, err error) {

	err = nil

	b, errFirst := contentReader.ReadByte()
	if errFirst != nil {
		err = fmt.Errorf("empty header")
		return
	}
	for {
		//uncomment to see raw characters
		//Debug(fmt.Sprint(int(rune(b))) + ": " + string(b))

		if (b == byte('\r')){
			b, err = contentReader.ReadByte()
			//Debug(fmt.Sprint(int(rune(b))) + ": " + string(b))
			if (b == byte('\n')) {
				//ok, we're done, we've scanned up to the \r\n successfully
				break
			} else {
				err = fmt.Errorf("missing LF after CR")
				return
			}

		} else {
			//keep this one
			header += string(b)
		}
		b, err = contentReader.ReadByte()
		if err != nil {
			err = fmt.Errorf("header too short")
			return
		}
	}

	_ , err2 := strconv.Atoi(string(header[0]))
	_ , err3 := strconv.Atoi(string(header[1]))
	if err2 != nil || err3 != nil {
		err = fmt.Errorf("first 2 characters must be digits")
		return
	}

	err = nil
	return
}
func (cgiHandler *cgiHandler) ServeGemini(w *Response, r *Request) {

	suffix := len(cgiHandler.cgiRoot)
	p := filepath.Clean(path.Join(cgiHandler.cgiRoot, r.URL.Path[2 + suffix:]))
	if !strings.HasPrefix(p, cgiHandler.cgiRoot) {
		w.SetStatus(StatusTemporaryFailure, "Path not in scope!")
		return
	}

	ServeCGI(p, w, r, *cgiHandler)
}

func cgiAllowed(fi os.FileInfo) bool {
	return uint64(fi.Mode().Perm())&0444 == 0444
}


