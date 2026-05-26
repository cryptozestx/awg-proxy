package proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

type HTTPProxyServer struct {
	server *http.Server
	dialer ContextDialer
}

func NewHTTPProxyServer(port int, dialer ContextDialer) (*HTTPProxyServer, int, error) {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, 0, err
	}
	actualPort := l.Addr().(*net.TCPAddr).Port

	p := &HTTPProxyServer{
		dialer: dialer,
	}

	p.server = &http.Server{
		Handler: p,
	}

	// Start listening in a background goroutine using the custom listener
	go func() {
		if err := p.server.Serve(l); err != nil && err != http.ErrServerClosed {
			log.Printf("[HTTP Proxy] Error: %v", err)
		}
	}()

	return p, actualPort, nil
}

func (p *HTTPProxyServer) Close() error {
	return p.server.Close()
}

func (p *HTTPProxyServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Log the incoming proxy requests for debugging
	// log.Printf("[HTTP Proxy] Request: %s %s", r.Method, r.URL.String())

	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}

	p.handleHTTP(w, r)
}

func (p *HTTPProxyServer) handleConnect(w http.ResponseWriter, r *http.Request) {
	// Hijack client connection
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "Hijacking not supported by the web server", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to hijack client connection: %v", err), http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// Dial the target through the netstack
	target := r.URL.Host
	if !strings.Contains(target, ":") {
		target = target + ":443" // Default HTTPS port
	}

	remoteConn, err := p.dialer.DialContext(context.Background(), "tcp", target)
	if err != nil {
		log.Printf("[HTTP Proxy] Failed to connect to %s: %v", target, err)
		_, _ = clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\n\r\n"))
		return
	}
	defer remoteConn.Close()

	// Notify client that connection is established
	_, err = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	if err != nil {
		return
	}

	// Relay data
	errChan := make(chan error, 2)
	go func() {
		_, err := io.Copy(clientConn, remoteConn)
		errChan <- err
	}()
	go func() {
		_, err := io.Copy(remoteConn, clientConn)
		errChan <- err
	}()

	<-errChan
}

func (p *HTTPProxyServer) handleHTTP(w http.ResponseWriter, r *http.Request) {
	// Prepare a transport that routes all outbound dials through the netstack
	transport := &http.Transport{
		DialContext:     p.dialer.DialContext,
		MaxIdleConns:    100,
		IdleConnTimeout: 90 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
	}

	// Construct the target URL request
	r.RequestURI = "" // Must be empty in client requests

	// Strip hop-by-hop headers
	stripHopByHop(r.Header)

	resp, err := client.Do(r)
	if err != nil {
		log.Printf("[HTTP Proxy] Request failed for %s: %v", r.URL.String(), err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	stripHopByHop(resp.Header)
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	_, _ = io.Copy(w, resp.Body)
}

func stripHopByHop(header http.Header) {
	hopHeaders := []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailers",
		"Transfer-Encoding",
		"Upgrade",
		"Proxy-Connection",
	}
	for _, h := range hopHeaders {
		header.Del(h)
	}
}
