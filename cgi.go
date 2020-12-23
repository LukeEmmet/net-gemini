package gemini

import (
	"bufio"
	"crypto/tls"
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

type cgiHandler struct {
	cgiRoot string
	bind string
}

func CGIServer(root string, bind string) Handler {
	return &cgiHandler{
		cgiRoot: filepath.Clean(root),
		bind: bind,
	}
}

func prepareCGIVariables(URL *url.URL, bind string, conn net.Conn, script_path string, path_info string) map[string]string {
	vars := prepareGatewayVariables( URL, bind, conn)
	vars["GATEWAY_INTERFACE"] = "CGI/1.1"
	vars["SCRIPT_PATH"] = script_path
	vars["PATH_INFO"] = path_info
	return vars
}

func prepareGatewayVariables(URL *url.URL, bind string, conn net.Conn) map[string]string {
	vars := make(map[string]string)
	vars["QUERY_STRING"] = URL.RawQuery
	vars["REMOTE_ADDR"] = conn.RemoteAddr().String()
	vars["REQUEST_METHOD"] = ""

	splitParts := strings.Split(bind, ":")
	vars["SERVER_NAME"] = splitParts[0]
	vars["SERVER_PORT"] = splitParts[1]
	vars["SERVER_PROTOCOL"] = "GEMINI"
	vars["SERVER_SOFTWARE"] = "GEMINIGEM"

	// Add TLS variables
	var tlsConn (*tls.Conn) = conn.(*tls.Conn)
	connState := tlsConn.ConnectionState()
	//	vars["TLS_CIPHER"] = CipherSuiteName(connState.CipherSuite)

	// Add client cert variables
	clientCerts := connState.PeerCertificates
	if len(clientCerts) > 0 {
		cert := clientCerts[0]
		//vars["TLS_CLIENT_HASH"] = getCertFingerprint(cert)
		vars["TLS_CLIENT_ISSUER"] = cert.Issuer.String()
		vars["TLS_CLIENT_ISSUER_CN"] = cert.Issuer.CommonName
		vars["TLS_CLIENT_SUBJECT"] = cert.Subject.String()
		vars["TLS_CLIENT_SUBJECT_CN"] = cert.Subject.CommonName
	}
	return vars
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

func ServeCGI(p string, w *Response, r *Request, bind string) {
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

	//Debug("ready to launch CGI")
	// Spawn process
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, exePath)		//unix style - just execute the script
	if runtime.GOOS == "windows" {
		//assume it is a script - read the first line to get the interpreter
		exePath, exeFlags = getShebang(p)
		cmd = exec.CommandContext(ctx, exePath, exeFlags, p)
	}

	//Debug("prepare CGI vars")
	// Prepare environment variables
	vars := prepareCGIVariables(URL, bind, w.conn, exePath, p)
	cmd.Env = []string{}


	for key, value := range vars {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	//Debug("launch CGI")
	response, err := cmd.Output()
	//Debug("CGI output: " + string(response))

	//fmt.Printf("%s", response)

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
	header, _, err := contentReader.ReadLine()
	_ , err2 := strconv.Atoi(strings.Fields(string(header))[0])
	if err != nil || err2 != nil {
		//first line did not start with a digit, or mising line ending - invalid
		w.SetStatus(StatusCGIError, "CGI error - invalid header!")
		return
	}


	//split the header into status and text
	statusSplit := strings.Split(string(header), " ")

	statusText := strings.Join(statusSplit[1:], " ")
	statusInt, err := strconv.Atoi(statusSplit[0])
	if err != nil {
		w.SetStatus(StatusCGIError, "CGI error - invalid status!")
	}

	//for some reason, setting w.status and w.statuscode - these arent writeable, need to set explicitly thus
	w.SetStatus(Status(statusInt), statusText)

	//the body is everything beyond the header text + /r/n
	w.Write(response[2 + len(header):])
}

func (fh *cgiHandler) ServeGemini(w *Response, r *Request) {

	suffix := len(fh.cgiRoot)
	p := filepath.Clean(path.Join(fh.cgiRoot, r.URL.Path[2 + suffix:]))
	if !strings.HasPrefix(p, fh.cgiRoot) {
		w.SetStatus(StatusTemporaryFailure, "Path not in scope!")
		return
	}

	ServeCGI(p, w, r, fh.bind)
}

func cgiAllowed(fi os.FileInfo) bool {
	return uint64(fi.Mode().Perm())&0444 == 0444
}


